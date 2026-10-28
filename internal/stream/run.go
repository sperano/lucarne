package stream

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// Update is one message of a run's stream, encoded once and shared by
// every subscriber: it must not be modified.
type Update struct {
	// Event is the SSE event name (contract.SSETranscript, ...).
	Event string
	// ID is the SSE id, the update's cursor; empty for a reset or live.
	ID string
	// Data is the message's JSON document.
	Data []byte
	seq  int64
}

// run is the live state of one run. Everything under mu changes together
// with seq, so that a snapshot and a subscription agree on a position.
type run struct {
	cfg    Config
	key    contract.RunKey
	epoch  string
	ctx    context.Context
	cancel context.CancelFunc
	// changed wakes the tracker when a read ends.
	changed chan struct{}
	// settled is closed once the first read of every attempt that was
	// terminated when the run was opened ended.
	settled chan struct{}
	readers sync.WaitGroup

	mu       sync.Mutex
	seq      int64
	src      kube.Run
	attempts []*attempt
	// forgotten are the attempts dropped past contract.MaxAttemptsPerRun.
	forgotten map[string]bool
	settling  int
	retained  retained
	replay    replay
	sources   sources
	subs      map[*Subscription]struct{}
	// idle is when the last subscriber left, zero while one is subscribed.
	idle time.Time
	// ended is the end update, once published: nothing follows it.
	ended *Update
}

func newRun(h *Hub, src kube.Run, listed []contract.Attempt) *run {
	ctx, cancel := context.WithCancel(h.ctx)
	r := &run{
		cfg: h.cfg, key: src.Key, epoch: newEpoch(), ctx: ctx, cancel: cancel,
		changed: make(chan struct{}, 1), settled: make(chan struct{}),
		src: src, subs: map[*Subscription]struct{}{}, idle: time.Now(), forgotten: map[string]bool{},
	}
	r.retained.bounds, r.replay.bounds, r.sources.bounds = h.cfg.Bounds, h.cfg.Bounds, h.cfg.Bounds
	for _, l := range listed {
		a := newAttempt(l)
		// The initial attempts are the snapshot's state at seq 0.
		a.published = a.current()
		if a.readable() && l.Container.State == contract.ContainerTerminated {
			a.settles = true
			r.settling++
		}
		r.attempts = append(r.attempts, a)
	}
	if r.settling == 0 {
		close(r.settled)
	}
	return r
}

// settle waits, bounded, for the logs of the attempts that had terminated
// when the run was opened, so that the first snapshot of a finished run
// holds its transcript.
func (r *run) settle(ctx context.Context) {
	timer := time.NewTimer(r.cfg.SettleTimeout)
	defer timer.Stop()
	select {
	case <-r.settled:
	case <-timer.C:
	case <-ctx.Done():
	case <-r.ctx.Done():
	}
}

func (r *run) settledOneLocked(a *attempt) {
	if !a.settles {
		return
	}
	a.settles = false
	if r.settling--; r.settling == 0 {
		close(r.settled)
	}
}

// stop stops following the run and ends its streams.
func (r *run) stop() {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs {
		r.dropLocked(s, nil)
	}
}

// idleSince is when the run was last watched, zero while it is.
func (r *run) idleSince() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.idle
}

// touchLocked restarts the idle time of a run nobody is subscribed to.
func (r *run) touchLocked() {
	if len(r.subs) == 0 {
		r.idle = time.Now()
	}
}

// source is the run as last read from the source.
func (r *run) source() kube.Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src
}

// snapshotDocument is contract.Snapshot with its events already encoded.
type snapshotDocument struct {
	Schema   string             `json:"schema"`
	Run      contract.Run       `json:"run"`
	Attempts []contract.Attempt `json:"attempts"`
	Events   []json.RawMessage  `json:"events"`
	Stream   contract.Stream    `json:"stream"`
	Limits   contract.Limits    `json:"limits"`
}

// snapshot encodes the run's state. Its lastSeq never counts the end
// update, which the stream after it replays.
func (r *run) snapshot() ([]byte, error) {
	r.mu.Lock()
	doc := snapshotDocument{
		Schema:   contract.SchemaVersion,
		Run:      r.src.Run,
		Attempts: make([]contract.Attempt, 0, len(r.attempts)),
		Events:   r.retained.documents(),
		Stream:   contract.Stream{Epoch: r.epoch, LastSeq: r.seq, DroppedEvents: r.retained.dropped},
		Limits:   r.cfg.Bounds.limits(),
	}
	if r.ended != nil {
		doc.Stream.LastSeq = r.ended.seq - 1
	}
	for _, a := range r.attempts {
		doc.Attempts = append(doc.Attempts, a.published)
	}
	r.touchLocked()
	r.mu.Unlock()
	return json.Marshal(doc)
}

// publishLocked encodes the next update and hands it to every subscriber.
// doc builds its document for its seq. Nothing is published after the end.
func (r *run) publishLocked(event string, doc func(seq int64) any) *Update {
	if r.ended != nil {
		return nil
	}
	seq := r.seq + 1
	data, err := json.Marshal(doc(seq))
	if err != nil {
		// The contract's types always encode: this is a programmer error,
		// dropped rather than leaving a hole in the seqs.
		r.cfg.Logger.Error("encode update", "run", r.key.LinkPath(), "event", event, "error", err)
		return nil
	}
	r.seq = seq
	u := &Update{Event: event, ID: Cursor{Epoch: r.epoch, Seq: seq}.String(), Data: data, seq: seq}
	r.replay.add(u)
	for s := range r.subs {
		select {
		case s.updates <- u:
		default:
			r.dropLocked(s, r.resetLocked(contract.ResetClientTooSlow))
		}
	}
	return u
}

func (r *run) publishEventLocked(ev contract.Event) {
	u := r.publishLocked(contract.SSETranscript, func(seq int64) any {
		ev.Seq = seq
		return ev
	})
	if u != nil {
		r.retained.add(u)
	}
}

func (r *run) publishRunLocked() {
	state := r.src.Run
	r.publishLocked(contract.SSERun, func(seq int64) any { return contract.RunUpdate{Seq: seq, Run: state} })
}

// publishAttemptsLocked sends the attempts whose state changed since they
// were last sent.
func (r *run) publishAttemptsLocked() {
	for _, a := range r.attempts {
		current := a.current()
		if equal(current, a.published) {
			continue
		}
		a.published = current
		r.publishLocked(contract.SSEAttempt, func(seq int64) any { return contract.AttemptUpdate{Seq: seq, Attempt: current} })
	}
}

func (r *run) publishEndLocked(reason contract.EndReason) {
	r.ended = r.publishLocked(contract.SSEEnd, func(seq int64) any { return contract.End{Seq: seq, Reason: reason} })
}

// resetLocked is a reset message of the current epoch.
func (r *run) resetLocked(reason contract.ResetReason) *Update {
	return unnumbered(contract.SSEReset, contract.Reset{Epoch: r.epoch, Reason: reason})
}

// unnumbered encodes a message that has no seq and no id.
func unnumbered(event string, doc any) *Update {
	data, err := json.Marshal(doc)
	if err != nil {
		panic("stream: encode " + event + ": " + err.Error())
	}
	return &Update{Event: event, Data: data}
}
