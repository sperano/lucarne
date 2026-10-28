package stream

import (
	"errors"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
)

// Gap details of the reads again of a log.
const (
	// DetailNotLinedUp is a log read again in which the last line read
	// before was not found where it should be.
	DetailNotLinedUp = "the log was read again and the last line read before was not found"
)

// errHalted stops the read of a halted reader.
var errHalted = errors.New("stream: reader halted")

// logSink turns the lines of one attempt's reads into transcript events:
// it skips what an earlier read delivered, numbers the gaps, and runs the
// rest through the runner's adapter. Its reader owns it; it takes the
// run's lock to publish.
type logSink struct {
	r      *run
	a      *attempt
	runner contract.RunnerKind
	// origin is the origin of the lines delivered, norm their adapter.
	origin string
	norm   *adapter.Normalizer
	align  alignment
	// gapLine is the line the last gap was before, gapPart the number of
	// gaps before it so far.
	gapLine int64
	gapPart int
	// expiredAt is the line an attempt_expired gap was before, 0 for none.
	expiredAt int64
}

func newLogSink(r *run, a *attempt, runner contract.RunnerKind) *logSink {
	return &logSink{r: r, a: a, runner: runner}
}

// Line implements kube.LogSink.
func (s *logSink) Line(origin string, line adapter.Line) error {
	events := s.useOrigin(origin)
	number, lost := s.align.place(line)
	if number != 0 {
		line.Number = number
		events = append(events, s.norm.Line(line)...)
	}
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.a.halted {
		return errHalted
	}
	if lost {
		s.gapLocked(contract.Gap{Reason: contract.GapReadInterrupted, Detail: DetailNotLinedUp}, number)
	}
	if number != 0 && line.Bytes > 0 {
		s.r.sources.add(s.a.key.ID(), s.origin, line)
	}
	for _, ev := range events {
		s.r.publishEventLocked(ev)
	}
	if number != 0 {
		s.a.lines++
		if line.Bytes > contract.MaxLineBytes {
			s.a.truncated++
		}
	}
	return nil
}

// Gap implements kube.LogSink.
func (s *logSink) Gap(origin string, next int64, gap contract.Gap) error {
	events := s.useOrigin(origin)
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.a.halted {
		return errHalted
	}
	for _, ev := range events {
		s.r.publishEventLocked(ev)
	}
	if before := s.align.gapBefore(next); before != 0 {
		s.gapLocked(gap, before)
	}
	return nil
}

// useOrigin switches to the lines of another origin: a first read, or a
// log whose start is gone (the source reports a source_truncated gap).
// The lines of the new origin are numbered from 1 and parsed afresh; it
// returns what the adapter still held of the old one.
func (s *logSink) useOrigin(origin string) []contract.Event {
	if origin == s.origin {
		return nil
	}
	var held []contract.Event
	if s.norm != nil {
		held = s.norm.End()
	}
	s.origin = origin
	s.norm = adapter.New(s.a.key.ID(), origin, newRunner(s.runner))
	s.align = alignment{aligned: true}
	s.gapLine, s.gapPart, s.expiredAt = 0, 0, 0
	return held
}

// gapLocked publishes a gap before line, numbered after the gaps already
// before it.
func (s *logSink) gapLocked(gap contract.Gap, line int64) {
	if line != s.gapLine {
		s.gapLine, s.gapPart = line, 0
	}
	part := s.gapPart
	s.gapPart++
	if gap.Reason == contract.GapAttemptExpired {
		s.expiredAt = line
	}
	id := s.a.key.ID()
	s.r.publishEventLocked(contract.Event{
		ID:      contract.GapEventID(id, s.origin, line, part),
		Attempt: id,
		Kind:    contract.KindGap,
		Source:  contract.Source{Line: line, Part: part},
		Gap:     &gap,
	})
	s.a.gaps++
}

// endReadLocked closes a read that ended as res says. A read that passed
// the last line delivered without finding it again, or a log gone without
// the source saying so after the last line delivered, leaves lines
// missing: a gap. When the log is done with, what the adapter held is
// delivered.
func (s *logSink) endReadLocked(res kube.ReadResult, final bool) {
	if s.origin == "" {
		return
	}
	next := s.align.delivered + 1
	if s.align.unresolved() {
		s.gapLocked(contract.Gap{Reason: contract.GapReadInterrupted, Detail: DetailNotLinedUp}, next)
	}
	if res.End == kube.EndExpired && s.expiredAt != next {
		s.gapLocked(contract.Gap{Reason: contract.GapAttemptExpired, Detail: cutLabel(res.Reason)}, next)
	}
	if final {
		for _, ev := range s.norm.End() {
			s.r.publishEventLocked(ev)
		}
	}
}

// cutLabel bounds a reason to contract.MaxLabelBytes.
func cutLabel(s string) string {
	cut, _ := adapter.Cut(s, contract.MaxLabelBytes)
	return cut
}
