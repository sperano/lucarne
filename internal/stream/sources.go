package stream

import (
	"context"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// ErrSourceNotFound is an event whose line the run does not hold: a gap,
// a line dropped to stay within the bounds, or one of another run or read.
var ErrSourceNotFound = errors.New("stream: no such source line")

// lineKey names one line of one read of an attempt's log, as an event ID
// does.
type lineKey struct {
	attempt, origin string
	line            int64
}

// sourceLine is a line kept for the events it produced.
type sourceLine struct {
	key   lineKey
	text  string
	bytes int64
}

// sources are the log lines the run's events came from: at most
// Bounds.EventsPerRun of them, in at most Bounds.SourceBytes, the oldest
// dropped first. A line is kept once, however many events it produced.
type sources struct {
	bounds Bounds
	lines  deque[*sourceLine]
	byKey  map[lineKey]*sourceLine
	bytes  int64
}

// add keeps the line number of attempt's read from origin, its text cut to
// contract.MaxSourceBytes.
func (b *sources) add(attempt, origin string, line adapter.Line) {
	// Only the start of a long line is converted: at most a rune past the
	// limit, which Cut then cuts at a rune boundary.
	head := line.Text[:min(len(line.Text), contract.MaxSourceBytes+utf8.UTFMax)]
	text, _ := adapter.Cut(string(head), contract.MaxSourceBytes)
	l := &sourceLine{key: lineKey{attempt, origin, line.Number}, text: text, bytes: line.Bytes}
	if b.byKey == nil {
		b.byKey = map[lineKey]*sourceLine{}
	}
	b.byKey[l.key] = l
	b.lines.push(l)
	b.bytes += int64(len(l.text))
	for b.lines.len() > 0 && (b.lines.len() > b.bounds.EventsPerRun || b.bytes > b.bounds.SourceBytes) {
		old := b.lines.pop()
		b.bytes -= int64(len(old.text))
		if b.byKey[old.key] == old {
			delete(b.byKey, old.key)
		}
	}
}

// find is the line an event ID names, nil when none is held.
func (b *sources) find(ref contract.EventRef) *sourceLine {
	if ref.Gap {
		return nil
	}
	return b.byKey[lineKey{ref.Attempt, ref.Origin, ref.Line}]
}

// sourceDocument encodes the line ref names: contract.SourceLine.
func (r *run) sourceDocument(ref contract.EventRef) ([]byte, error) {
	r.mu.Lock()
	l := r.sources.find(ref)
	r.touchLocked()
	r.mu.Unlock()
	if l == nil {
		return nil, ErrSourceNotFound
	}
	return json.Marshal(contract.SourceLine{
		Schema:  contract.SchemaVersion,
		Attempt: l.key.attempt,
		Origin:  l.key.origin,
		Line:    l.key.line,
		Text:    l.text,
		Bytes:   l.bytes,
	})
}

// Source is the line of the run with key that the event ref names came
// from, encoded: contract.SourceLine. The run is resolved as for Snapshot,
// and only the lines read for it are looked in.
func (h *Hub) Source(ctx context.Context, key contract.RunKey, ref contract.EventRef) ([]byte, error) {
	r, fresh, err := h.acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	if fresh {
		r.settle(ctx)
	}
	return r.sourceDocument(ref)
}
