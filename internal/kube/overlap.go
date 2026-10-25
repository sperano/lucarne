package kube

import (
	"crypto/sha256"
	"time"
)

// maxOverlapLines bounds the lines a reconnect must find again: those read
// in the second of the last line, which a read since that second returns
// first. Past it, only the newest are matched.
const maxOverlapLines = 1024

// tail is the end of what a read delivered, kept to resume a stream that
// broke: the lines delivered in the second of the last one.
type tail struct {
	sums [][sha256.Size]byte
	// since is the second of the last line, last its time.
	since, last time.Time
	// untimed is a line delivered without a timestamp: a read since a
	// second cannot place it, so nothing can be resumed. The log API
	// writes a timestamp on every line asked for one, so it is sticky.
	untimed bool
}

func (t *tail) empty() bool { return len(t.sums) == 0 && !t.untimed }

// add records a delivered line.
func (t *tail) add(l rawLine) {
	if l.time == nil {
		t.untimed = true
		return
	}
	if second := l.time.Truncate(time.Second); second.After(t.since) {
		t.sums, t.since = t.sums[:0], second
	}
	t.sums = append(t.sums, l.sum)
	if extra := len(t.sums) - maxOverlapLines; extra > 0 {
		t.sums = append(t.sums[:0], t.sums[extra:]...)
	}
	if l.time.After(t.last) {
		t.last = *l.time
	}
}

// overlapStep is what a line of a resumed stream is.
type overlapStep int

const (
	// overlapSkip is a line delivered before the break.
	overlapSkip overlapStep = iota
	// overlapFound is the last line delivered before the break: the next
	// line is new.
	overlapFound
	// overlapLost is a new line found before the tail was: lines between
	// the break and it may be missing.
	overlapLost
)

// resume finds a tail again at the start of a stream read since its second.
type resume struct {
	tail    *tail
	matched int
}

func newResume(t *tail) *resume { return &resume{tail: t} }

// step classifies the next line of the resumed stream. The tail's lines
// must come in order; a line not in it, older than the last, is one
// delivered before the tail started, from the same second.
func (r *resume) step(l rawLine) overlapStep {
	sums := r.tail.sums
	if l.time == nil {
		return overlapLost
	}
	if l.sum == sums[r.matched] {
		r.matched++
		if r.matched == len(sums) {
			return overlapFound
		}
		return overlapSkip
	}
	if l.time.After(r.tail.last) {
		return overlapLost
	}
	r.matched = 0
	if l.sum == sums[0] {
		r.matched = 1
		if len(sums) == 1 {
			return overlapFound
		}
	}
	return overlapSkip
}
