package stream

import (
	"context"
	"errors"
	"time"

	"github.com/sperano/lucarne/internal/kube"
)

// ReasonAttemptGone is the coverage reason of an attempt the source no
// longer finds: its pod now runs another instance of the container.
const ReasonAttemptGone = "attempt no longer found in its pod"

// startReaderLocked starts following a's log, when it has one to read.
func (r *run) startReaderLocked(a *attempt) {
	if !a.readable() || a.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.ctx)
	a.cancel = cancel
	a.sink = newLogSink(r, a, r.src.Runner)
	r.readers.Add(1)
	go func() {
		defer r.readers.Done()
		defer cancel()
		r.follow(ctx, a)
	}()
}

// haltLocked stops a's reader; nothing it still reads is delivered.
func (r *run) haltLocked(a *attempt) {
	a.halted = true
	if a.cancel != nil {
		a.cancel()
	}
}

// stopReaders stops every reader and waits for them.
func (r *run) stopReaders() {
	r.mu.Lock()
	for _, a := range r.attempts {
		r.haltLocked(a)
	}
	r.mu.Unlock()
	r.readers.Wait()
}

// follow reads a's log until it is read to its end or gone, reading it
// again from its start, after a delay, whenever it could not be read.
func (r *run) follow(ctx context.Context, a *attempt) {
	for {
		if !r.readStarting(a) {
			return
		}
		opts := kube.ReadOptions{Follow: true, KnownOrigin: a.sink.origin}
		res, err := r.cfg.Source.ReadLog(ctx, r.source(), a.key, opts, a.sink)
		if r.readEnded(a, res, err) || wait(ctx, r.cfg.RereadDelay) != nil {
			return
		}
	}
}

// readStarting records a read about to start, false when a was halted.
func (r *run) readStarting(a *attempt) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.halted {
		r.settledOneLocked(a)
		return false
	}
	a.reading, a.read = true, true
	a.sink.align.startRead()
	r.publishAttemptsLocked()
	return true
}

// readEnded records how a read ended and reports whether the log is done
// with: read to its end, gone, or a halted.
func (r *run) readEnded(a *attempt, res kube.ReadResult, err error) bool {
	end, reason := res.End, res.Reason
	switch {
	case errors.Is(err, kube.ErrAttemptNotFound):
		end, reason = kube.EndExpired, ReasonAttemptGone
	case err != nil && !errors.Is(err, errHalted) && !errors.Is(err, context.Canceled):
		r.cfg.Logger.Warn("read log", "run", r.key.LinkPath(), "attempt", a.key.ID(), "error", err)
		if end == "" {
			end, reason = kube.EndUnavailable, kube.ReasonSourceUnavailable
		}
	}
	defer r.signal()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settledOneLocked(a)
	a.reading = false
	if a.halted {
		return true
	}
	final := finalEnd(end)
	a.sink.endReadLocked(kube.ReadResult{End: end, Reason: reason}, final)
	a.end, a.reason = end, reason
	r.publishAttemptsLocked()
	return final
}

// wait sleeps for d, or returns the context's error.
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
