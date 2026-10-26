package stream

import (
	"time"

	"github.com/sperano/lucarne/contract"
)

// Subscription is one client's stream of a run. Exactly one of Reset,
// Finished and Replay applies:
//
//   - Reset set: send it, and nothing else; the client loads a snapshot.
//   - Finished: the cursor is at or after the run's end; nothing is left
//     to send, and the client must not reconnect.
//   - otherwise: send Replay, the updates after the cursor (which stop at
//     an end update when the run has ended), then Live, then each update of
//     Updates until Done is closed.
//
// Close it when the client is gone.
type Subscription struct {
	Reset    *Update
	Finished bool
	Replay   []*Update
	Live     *Update

	run     *run
	updates chan *Update
	stop    chan struct{}
	// err is the reset to send once stop is closed, nil when the server
	// ends the stream.
	err *Update
}

// Updates delivers the updates published after the subscription, in order.
func (s *Subscription) Updates() <-chan *Update { return s.updates }

// Done is closed when the stream must end: Err says how.
func (s *Subscription) Done() <-chan struct{} { return s.stop }

// Err is, once Done is closed, the reset to send before closing the
// stream: client_too_slow when the client fell more than
// Bounds.ClientQueue updates behind; nil when the server stops the stream.
func (s *Subscription) Err() *Update { return s.err }

// Close releases the subscription.
func (s *Subscription) Close() {
	if s.run != nil {
		s.run.unsubscribe(s)
	}
}

// subscribe opens a stream after the cursor text. The replay and the
// registration happen under the lock that publishing takes, so the client
// gets every update after its cursor exactly once.
func (r *run) subscribe(text string) *Subscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.touchLocked()
	cursor, ok := ParseCursor(text)
	switch {
	case !ok:
		return &Subscription{Reset: r.resetLocked(contract.ResetBadCursor)}
	case cursor.Epoch != r.epoch:
		return &Subscription{Reset: r.resetLocked(contract.ResetEpochChanged)}
	case r.ended != nil && cursor.Seq >= r.ended.seq:
		// Nothing follows the end, so a cursor after it is finished too.
		return &Subscription{Finished: true}
	case cursor.Seq > r.seq:
		return &Subscription{Reset: r.resetLocked(contract.ResetBadCursor)}
	}
	updates, ok := r.replay.after(cursor.Seq, r.seq)
	if !ok {
		return &Subscription{Reset: r.resetLocked(contract.ResetReplayExpired)}
	}
	s := &Subscription{
		Replay:  updates,
		Live:    unnumbered(contract.SSELive, contract.Live{LastSeq: r.seq}),
		run:     r,
		updates: make(chan *Update, r.cfg.Bounds.ClientQueue),
		stop:    make(chan struct{}),
	}
	if r.ended != nil || r.ctx.Err() != nil {
		// The replay holds the end, or the run is stopped: nothing will
		// follow, and the client of a stopped run reconnects to another
		// epoch.
		close(s.stop)
		return s
	}
	r.subs[s] = struct{}{}
	r.idle = time.Time{}
	return s
}

func (r *run) unsubscribe(s *Subscription) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subs[s]; ok {
		delete(r.subs, s)
		r.touchLocked()
	}
}

// dropLocked ends a subscription: err is the reset it gets, nil for none.
func (r *run) dropLocked(s *Subscription, err *Update) {
	delete(r.subs, s)
	s.err = err
	close(s.stop)
	r.touchLocked()
}
