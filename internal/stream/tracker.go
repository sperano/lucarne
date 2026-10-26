package stream

import (
	"errors"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// track follows the run until it ends or is stopped: it reads the Task and
// its attempts every poll interval, and looks again whenever a read ends.
func (r *run) track() {
	defer r.stopReaders()
	r.mu.Lock()
	for _, a := range r.attempts {
		r.startReaderLocked(a)
	}
	r.mu.Unlock()
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if r.poll() {
				return
			}
		case <-r.changed:
			if r.check() {
				return
			}
		}
	}
}

// signal wakes the tracker.
func (r *run) signal() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

// poll reads the Task and its attempts again and reports whether the run
// ended. A source that fails keeps the last state until the next poll.
func (r *run) poll() bool {
	src, err := r.cfg.Source.Refresh(r.ctx, r.source())
	switch {
	case r.ctx.Err() != nil:
		return true
	case errors.Is(err, kube.ErrRunDeleted):
		r.deleted()
		return true
	case err != nil:
		r.cfg.Logger.Warn("refresh run", "run", r.key.LinkPath(), "error", err)
		return false
	}
	listed, err := r.cfg.Source.Attempts(r.ctx, src)
	if err != nil {
		r.cfg.Logger.Warn("list attempts", "run", r.key.LinkPath(), "error", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked(src)
	if err == nil {
		r.mergeLocked(listed)
	}
	return r.checkLocked()
}

func (r *run) check() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.checkLocked()
}

// checkLocked sends the attempts that changed, then the end of a finished
// run, and reports whether the run ended.
func (r *run) checkLocked() bool {
	r.publishAttemptsLocked()
	if r.ended == nil && r.finishedLocked() {
		r.publishEndLocked(contract.EndRunFinished)
	}
	return r.ended != nil
}

// finishedLocked is a run in a terminal phase whose attempts' logs will
// not change. A retry can still start while the phase is not terminal.
func (r *run) finishedLocked() bool {
	if phase := r.src.Phase; phase != contract.PhaseSucceeded && phase != contract.PhaseFailed {
		return false
	}
	for _, a := range r.attempts {
		if !a.final() {
			return false
		}
	}
	return true
}

func (r *run) refreshLocked(src kube.Run) {
	changed := !equal(src.Run, r.src.Run)
	r.src = src
	if changed {
		r.publishRunLocked()
	}
}

// mergeLocked takes the attempts the source lists now. A new one is added
// and read; one the source no longer lists (its pod deleted) is kept, and
// its read finds out. The oldest go past contract.MaxAttemptsPerRun, for
// good: the source may list them again.
func (r *run) mergeLocked(listed []contract.Attempt) {
	known := make(map[string]*attempt, len(r.attempts))
	for _, a := range r.attempts {
		known[a.key.ID()] = a
	}
	for _, l := range listed {
		if a, ok := known[l.ID]; ok {
			a.listed = l
			continue
		}
		if r.forgotten[l.ID] {
			continue
		}
		a := newAttempt(l)
		r.attempts = append(r.attempts, a)
		r.startReaderLocked(a)
	}
	if extra := len(r.attempts) - contract.MaxAttemptsPerRun; extra > 0 {
		for _, a := range r.attempts[:extra] {
			r.haltLocked(a)
			r.forgotten[a.key.ID()] = true
		}
		r.attempts = append([]*attempt(nil), r.attempts[extra:]...)
	}
}

// deleted ends a run whose Task is gone: its reads stop, the logs still
// being read are expired, with a gap where lines were read, and the
// stream ends.
func (r *run) deleted() {
	r.stopReaders()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.src.Deleted = true
	r.publishRunLocked()
	for _, a := range r.attempts {
		if a.final() {
			continue
		}
		if a.sink != nil {
			a.sink.endReadLocked(kube.ReadResult{End: kube.EndExpired, Reason: kube.ReasonRunDeleted}, true)
		}
		a.read, a.end, a.reason = true, kube.EndExpired, kube.ReasonRunDeleted
	}
	r.publishAttemptsLocked()
	r.publishEndLocked(contract.EndRunDeleted)
}
