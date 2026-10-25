// Package adapter turns the lines of one attempt's log into transcript
// events. It holds what every runner shares: event numbering, the
// contract's bounds, the raw fallback and the Kelos marker lines. Each
// runner's own output format is a Runner in a subpackage (claude).
//
// An adapter reads nothing itself: the log source hands it lines, numbered
// from the start of its read, and the stream assigns each event its seq.
// It makes no model call and needs no agent home or session directory.
package adapter

import "github.com/sperano/lucarne/contract"

// Line is one line of an attempt's log as the log source read it.
type Line struct {
	// Number is the line's 1-based ordinal in the log read, blank lines
	// included.
	Number int64
	// Text is the line without its ending or timestamp prefix. The source
	// holds at most contract.MaxLineBytes of it.
	Text []byte
	// Bytes is the length of the whole line: more than
	// contract.MaxLineBytes when the source could not hold all of it.
	Bytes int64
	// LogTime is the container runtime's receipt time of the line, nil
	// when the log was read without timestamps.
	LogTime *contract.Timestamp
}

// Runner normalizes the lines of one runner's own output: every line that
// is not blank, oversized or a Kelos marker.
type Runner interface {
	// Line appends the events one line produces to out.
	Line(text []byte, out *Emitter)
}

// Normalizer turns the lines of one attempt's log, in order, into events.
// It is not safe for concurrent use; a log is read by one goroutine.
type Normalizer struct {
	attempt string
	origin  string
	runner  Runner
	// outputs is the Kelos outputs block being read, nil outside one.
	outputs *outputsBlock
}

// New returns a Normalizer for the log of attempt read from origin (see
// contract.LogOrigin), whose runner output runner parses.
func New(attemptID, origin string, runner Runner) *Normalizer {
	return &Normalizer{attempt: attemptID, origin: origin, runner: runner}
}

// Line returns the events line produces, in order, with every field but
// Seq set. A line of a Kelos outputs block produces no event of its own:
// the block is one event, returned with the line that ends it.
func (n *Normalizer) Line(line Line) []contract.Event {
	out := n.emitter(line)
	switch {
	case line.Bytes > contract.MaxLineBytes:
		out.oversized()
	case len(line.Text) == 0:
		// A blank line produces no event; the source counted it.
	case n.kelos(out):
	default:
		n.runner.Line(line.Text, out)
	}
	return out.events
}

// End returns the events still held when the log ends: an outputs block
// whose end marker never came.
func (n *Normalizer) End() []contract.Event {
	return n.flushOutputs()
}

func (n *Normalizer) emitter(line Line) *Emitter {
	return &Emitter{attempt: n.attempt, origin: n.origin, line: line}
}
