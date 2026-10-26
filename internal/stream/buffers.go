package stream

import "encoding/json"

// retained are the transcript events a snapshot holds: at most
// Bounds.EventsPerRun of them, in at most Bounds.RunBytes, the oldest
// dropped first and counted.
type retained struct {
	bounds  Bounds
	events  deque[*Update]
	bytes   int64
	dropped int64
}

func (b *retained) add(u *Update) {
	b.events.push(u)
	b.bytes += int64(len(u.Data))
	for b.events.len() > 0 && (b.events.len() > b.bounds.EventsPerRun || b.bytes > b.bounds.RunBytes) {
		b.bytes -= int64(len(b.events.pop().Data))
		b.dropped++
	}
}

// documents are the retained events, oldest first.
func (b *retained) documents() []json.RawMessage {
	docs := make([]json.RawMessage, b.events.len())
	for i := range docs {
		docs[i] = b.events.at(i).Data
	}
	return docs
}

// replay are the last updates, which a reconnecting client is sent again:
// at most Bounds.ReplayUpdates of them, in at most Bounds.RunBytes. Their
// seqs are contiguous.
type replay struct {
	bounds  Bounds
	updates deque[*Update]
	bytes   int64
}

func (b *replay) add(u *Update) {
	b.updates.push(u)
	b.bytes += int64(len(u.Data))
	// The newest stays, whatever its size: the end must be replayable.
	for b.updates.len() > 1 && (b.updates.len() > b.bounds.ReplayUpdates || b.bytes > b.bounds.RunBytes) {
		b.bytes -= int64(len(b.updates.pop().Data))
	}
}

// after returns the updates after seq, up to last, the newest seq; false
// when some of them are no longer held.
func (b *replay) after(seq, last int64) ([]*Update, bool) {
	if seq == last {
		return nil, true
	}
	if b.updates.len() == 0 {
		return nil, false
	}
	first := b.updates.at(0).seq
	if seq+1 < first {
		return nil, false
	}
	updates := make([]*Update, 0, last-seq)
	for i := int(seq + 1 - first); i < b.updates.len(); i++ {
		updates = append(updates, b.updates.at(i))
	}
	return updates, true
}
