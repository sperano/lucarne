package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sperano/lucarne/contract"
)

// Defaults of Config.
const (
	// DefaultPollInterval is how often a followed run's Task and attempts
	// are read again.
	DefaultPollInterval = 2 * time.Second
	// DefaultRereadDelay is the wait before a log that could not be read
	// is read again.
	DefaultRereadDelay = 10 * time.Second
	// DefaultIdleTimeout is how long a run nobody watches is kept.
	DefaultIdleTimeout = 5 * time.Minute
	// DefaultSettleTimeout bounds how long the first snapshot of a run
	// waits for the logs of its terminated attempts to be read.
	DefaultSettleTimeout = 3 * time.Second
	// DefaultMaxRuns is how many runs a hub follows at once. A run holds
	// up to contract.MaxRunBytes of events, and as much of replay.
	DefaultMaxRuns = 16
)

// Errors of Snapshot and Subscribe besides the source's, which wrap
// kube.ErrRunNotFound, kube.ErrUnavailable or kube.ErrForbidden.
var (
	// ErrBusy is a new run while the hub follows as many as it may, all
	// of them watched.
	ErrBusy = errors.New("stream: following as many runs as allowed")
	// ErrClosed is a request after Close.
	ErrClosed = errors.New("stream: hub closed")
)

// Bounds are the stream limits a hub enforces; zero takes the contract's.
type Bounds struct {
	EventsPerRun  int
	RunBytes      int64
	ReplayUpdates int
	ClientQueue   int
}

func (b Bounds) withDefaults() Bounds {
	b.EventsPerRun = cmpOr(b.EventsPerRun, contract.MaxEventsPerRun)
	b.RunBytes = cmpOr(b.RunBytes, contract.MaxRunBytes)
	b.ReplayUpdates = cmpOr(b.ReplayUpdates, contract.MaxReplayUpdates)
	b.ClientQueue = cmpOr(b.ClientQueue, contract.MaxClientQueue)
	return b
}

// limits are the contract limits a snapshot publishes, with these bounds.
func (b Bounds) limits() contract.Limits {
	l := contract.DefaultLimits()
	l.MaxEventsPerRun, l.MaxRunBytes = int64(b.EventsPerRun), b.RunBytes
	l.MaxReplayUpdates, l.MaxClientQueue = int64(b.ReplayUpdates), int64(b.ClientQueue)
	return l
}

// Config is what a hub reads and how. Every zero duration or count takes
// its default.
type Config struct {
	Source        Source
	Logger        *slog.Logger
	PollInterval  time.Duration
	RereadDelay   time.Duration
	IdleTimeout   time.Duration
	SettleTimeout time.Duration
	MaxRuns       int
	Bounds        Bounds
}

func (c Config) withDefaults() Config {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	c.PollInterval = cmpOr(c.PollInterval, DefaultPollInterval)
	c.RereadDelay = cmpOr(c.RereadDelay, DefaultRereadDelay)
	c.IdleTimeout = cmpOr(c.IdleTimeout, DefaultIdleTimeout)
	c.SettleTimeout = cmpOr(c.SettleTimeout, DefaultSettleTimeout)
	c.MaxRuns = cmpOr(c.MaxRuns, DefaultMaxRuns)
	c.Bounds = c.Bounds.withDefaults()
	return c
}

// cmpOr is v, or def when v is not positive.
func cmpOr[T int | int64 | time.Duration](v, def T) T {
	if v <= 0 {
		return def
	}
	return v
}

// Hub holds the runs being followed. It is safe for concurrent use.
type Hub struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	runs   map[contract.RunKey]*entry
	closed bool
}

// entry is a run in the hub, or one being opened: ready is closed once run
// or err is set.
type entry struct {
	ready chan struct{}
	run   *run
	err   error
}

// New returns a hub reading from cfg.Source, which must be set.
func New(cfg Config) (*Hub, error) {
	if cfg.Source == nil {
		return nil, errors.New("stream: no source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Hub{cfg: cfg.withDefaults(), ctx: ctx, cancel: cancel, runs: map[contract.RunKey]*entry{}}
	h.wg.Add(1)
	go h.janitor()
	return h, nil
}

// Close stops following every run and ends every stream, without an end
// message: the client reconnects, to another epoch.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	runs := h.runs
	h.runs = map[contract.RunKey]*entry{}
	h.mu.Unlock()
	h.cancel()
	for _, e := range runs {
		<-e.ready
		if e.run != nil {
			e.run.stop()
		}
	}
	h.wg.Wait()
}

// Snapshot is the run's snapshot, encoded: contract.Snapshot.
func (h *Hub) Snapshot(ctx context.Context, key contract.RunKey) ([]byte, error) {
	r, fresh, err := h.acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	if fresh {
		r.settle(ctx)
	}
	return r.snapshot()
}

// Subscribe opens the run's stream after cursor, the text of a
// Last-Event-ID or after parameter, empty for none.
func (h *Hub) Subscribe(ctx context.Context, key contract.RunKey, cursor string) (*Subscription, error) {
	r, _, err := h.acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	return r.subscribe(cursor), nil
}

// acquire returns the run with key, opening it when the hub does not
// follow it yet; fresh is a run this call opened.
func (h *Hub) acquire(ctx context.Context, key contract.RunKey) (*run, bool, error) {
	for {
		h.mu.Lock()
		if h.closed {
			h.mu.Unlock()
			return nil, false, ErrClosed
		}
		if e, ok := h.runs[key]; ok {
			h.mu.Unlock()
			r, err := h.await(ctx, e)
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				// The request that opened it gave up; this one tries.
				continue
			}
			return r, false, err
		}
		if !h.roomLocked() {
			h.mu.Unlock()
			return nil, false, ErrBusy
		}
		e := &entry{ready: make(chan struct{})}
		h.runs[key] = e
		h.mu.Unlock()
		r, err := h.open(ctx, key)
		h.opened(key, e, r, err)
		return r, true, err
	}
}

func (h *Hub) await(ctx context.Context, e *entry) (*run, error) {
	select {
	case <-e.ready:
		return e.run, e.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// opened records the outcome of opening the run with key. A run that could
// not be opened is forgotten: the next request tries again.
func (h *Hub) opened(key contract.RunKey, e *entry, r *run, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		delete(h.runs, key)
	} else if h.closed {
		r.stop()
		err, r = ErrClosed, nil
	}
	e.run, e.err = r, err
	close(e.ready)
}

// roomLocked reports whether a run can be added, dropping the run idle the
// longest when the hub is full.
func (h *Hub) roomLocked() bool {
	if len(h.runs) < h.cfg.MaxRuns {
		return true
	}
	var oldest contract.RunKey
	var since time.Time
	for key, e := range h.runs {
		if !isReady(e) || e.run == nil {
			continue
		}
		if idle := e.run.idleSince(); !idle.IsZero() && (since.IsZero() || idle.Before(since)) {
			oldest, since = key, idle
		}
	}
	if since.IsZero() {
		return false
	}
	h.dropLocked(oldest)
	return true
}

func isReady(e *entry) bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

func (h *Hub) dropLocked(key contract.RunKey) {
	e := h.runs[key]
	delete(h.runs, key)
	go e.run.stop()
}

// janitor drops the runs nobody watched for the idle timeout.
func (h *Hub) janitor() {
	defer h.wg.Done()
	ticker := time.NewTicker(h.cfg.IdleTimeout / janitorTicks)
	defer ticker.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case now := <-ticker.C:
			h.dropIdle(now)
		}
	}
}

// janitorTicks is how many times per idle timeout the janitor looks.
const janitorTicks = 4

func (h *Hub) dropIdle(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, e := range h.runs {
		if !isReady(e) || e.run == nil {
			continue
		}
		if idle := e.run.idleSince(); !idle.IsZero() && now.Sub(idle) >= h.cfg.IdleTimeout {
			h.dropLocked(key)
		}
	}
}

// open resolves the run with key and starts following it.
func (h *Hub) open(ctx context.Context, key contract.RunKey) (*run, error) {
	// Close must not wait for a source that does not answer.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(h.ctx, cancel)()
	src, err := h.cfg.Source.Run(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("resolve run: %w", err)
	}
	attempts, err := h.cfg.Source.Attempts(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("list attempts: %w", err)
	}
	r := newRun(h, src, attempts)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		r.track()
	}()
	return r, nil
}
