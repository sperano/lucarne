package stream

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

// finished is a source of a succeeded run whose one attempt terminated,
// read with reads.
func finished(reads ...streamtest.Read) *streamtest.Source {
	src := source(contract.PhaseSucceeded, terminated())
	src.AddReads(attemptID, reads...)
	return src
}

// script is a read of steps from origin, then end: each step a line or a
// gap before the line numbered next.
func script(origin string, end kube.ReadEnd, steps ...any) streamtest.Read {
	return func(_ context.Context, sink kube.LogSink) (kube.ReadResult, error) {
		for _, step := range steps {
			var err error
			switch s := step.(type) {
			case adapter.Line:
				err = sink.Line(origin, s)
			case gapStep:
				err = sink.Gap(origin, s.next, contract.Gap{Reason: s.reason})
			}
			if err != nil {
				return kube.ReadResult{}, err
			}
		}
		return kube.ReadResult{Origin: origin, End: end}, nil
	}
}

type gapStep struct {
	next   int64
	reason contract.GapReason
}

// numbered is line, numbered as another read numbered it.
func numbered(line adapter.Line, number int64) adapter.Line {
	line.Number = number
	return line
}

func TestRereadSkipsTheOverlap(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("first")
	src := finished(broken(origin, 1, 3), complete(origin, 1, 5))
	snap := settled(t, newHub(t, src))
	want := []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`, `{"n":5}`}
	if got := texts(snap.Events); !slices.Equal(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
	for i, ev := range snap.Events {
		if want := contract.EventID(attemptID, origin, int64(i+1), 0); ev.ID != want {
			t.Errorf("event %d is %s, want %s", i, ev.ID, want)
		}
	}
	opens := src.Opens(attemptID)
	if len(opens) != 2 || opens[0].KnownOrigin != "" || opens[1].KnownOrigin != origin || !opens[1].Follow {
		t.Errorf("reads %+v, want a follow, then one knowing origin %s", opens, origin)
	}
	coverage := attemptOf(t, snap, attemptID).Coverage
	if coverage.State != contract.CoverageEnded || coverage.Lines != 5 || coverage.Gaps != 0 {
		t.Errorf("coverage %+v, want ended with 5 lines", coverage)
	}
}

func TestRereadKeepsRepeatedIdenticalLines(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]func(n int64) adapter.Line{
		"timed": func(n int64) adapter.Line { return streamtest.Line(n, "same") },
		// Without timestamps, only the number tells two identical lines apart.
		"untimed": func(n int64) adapter.Line { l := streamtest.Line(n, "same"); l.LogTime = nil; return l },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			origin := streamtest.Origin("same")
			first := script(origin, kube.EndUnavailable, line(1), line(2))
			again := script(origin, kube.EndComplete, line(1), line(2), line(3))
			snap := settled(t, newHub(t, finished(first, again)))
			if got := texts(snap.Events); !slices.Equal(got, []string{"same", "same", "same"}) {
				t.Errorf("texts %q, want three lines", got)
			}
		})
	}
}

func TestRereadLinesUpAfterLostLines(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("lost")
	all := streamtest.Lines(1, 6)
	// The first read lost lines 3 and 4 and numbered line 5 as 3.
	first := script(origin, kube.EndUnavailable, all[0], all[1], gapStep{3, contract.GapReadInterrupted}, numbered(all[4], 3))
	again := script(origin, kube.EndComplete, steps(all)...)
	snap := settled(t, newHub(t, finished(first, again)))
	want := []string{`{"n":1}`, `{"n":2}`, `{"n":5}`, `{"n":6}`}
	if got := texts(snap.Events); !slices.Equal(got, want) {
		t.Errorf("texts %q, want %q", got, want)
	}
	if last := snap.Events[len(snap.Events)-1]; last.Source.Line != 4 {
		t.Errorf("line 6 of the re-read numbered %d, want 4", last.Source.Line)
	}
}

// steps are lines as the steps of a script.
func steps(lines []adapter.Line) []any {
	out := make([]any, len(lines))
	for i, l := range lines {
		out[i] = l
	}
	return out
}

func TestRereadThatCannotLineUpIsAGap(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("other")
	// other are lines numbered as the first read's, with other content.
	other := func(count int64) []any {
		var lines []adapter.Line
		for n := int64(1); n <= count; n++ {
			lines = append(lines, streamtest.Line(n, fmt.Sprintf("other %d", n)))
		}
		return steps(lines)
	}
	tests := map[string]struct {
		again     streamtest.Read
		wantLines int64
	}{
		// Past maxRealignLines the rest is delivered after a gap.
		"window passed": {script(origin, kube.EndComplete, other(maxRealignLines+4)...), 3 + 3},
		// A read that ends first leaves a gap after what was delivered.
		"read ended": {script(origin, kube.EndComplete, other(5)...), 3},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := settled(t, newHub(t, finished(broken(origin, 1, 3), tt.again)))
			var gaps []contract.Event
			for _, ev := range snap.Events {
				if ev.Kind == contract.KindGap {
					gaps = append(gaps, ev)
				}
			}
			if len(gaps) != 1 || gaps[0].Gap.Reason != contract.GapReadInterrupted || gaps[0].Gap.Detail != DetailNotLinedUp || gaps[0].Source.Line != 4 {
				t.Fatalf("gaps %+v, want one read_interrupted before line 4", gaps)
			}
			coverage := attemptOf(t, snap, attemptID).Coverage
			if coverage.Lines != tt.wantLines || coverage.Gaps != 1 {
				t.Errorf("coverage %+v, want %d lines and a gap", coverage, tt.wantLines)
			}
		})
	}
}

func TestRotatedLogIsReadFromItsNewOrigin(t *testing.T) {
	t.Parallel()
	before, after := streamtest.Origin("before"), streamtest.Origin("after")
	rotated := script(after, kube.EndComplete,
		gapStep{1, contract.GapSourceTruncated}, streamtest.Line(1, "kept"), streamtest.Line(2, "new"))
	src := finished(broken(before, 1, 2), rotated)
	snap := settled(t, newHub(t, src))
	want := []string{
		contract.EventID(attemptID, before, 1, 0),
		contract.EventID(attemptID, before, 2, 0),
		contract.GapEventID(attemptID, after, 1, 0),
		contract.EventID(attemptID, after, 1, 0),
		contract.EventID(attemptID, after, 2, 0),
	}
	if got := ids(snap.Events); !slices.Equal(got, want) {
		t.Errorf("events %q, want %q", got, want)
	}
	if opens := src.Opens(attemptID); opens[1].KnownOrigin != before {
		t.Errorf("re-read knew origin %q, want %q", opens[1].KnownOrigin, before)
	}
}

func TestLogGoneAfterABrokenReadIsAGap(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("gone")
	gone := streamtest.Deliver("", nil, kube.EndExpired, kube.ReasonPodDeleted)
	snap := settled(t, newHub(t, finished(broken(origin, 1, 2), gone)))
	last := snap.Events[len(snap.Events)-1]
	if last.ID != contract.GapEventID(attemptID, origin, 3, 0) || last.Gap.Reason != contract.GapAttemptExpired || last.Gap.Detail != kube.ReasonPodDeleted {
		t.Errorf("last event %+v, want an attempt_expired gap before line 3", last)
	}
	coverage := attemptOf(t, snap, attemptID).Coverage
	if coverage.State != contract.CoverageExpired || coverage.Reason != kube.ReasonPodDeleted || coverage.Gaps != 1 {
		t.Errorf("coverage %+v, want expired with one gap", coverage)
	}
}

func TestExpiredGapIsNotRepeated(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("expired")
	read := script(origin, kube.EndExpired, streamtest.Line(1, "a"), gapStep{2, contract.GapAttemptExpired})
	snap := settled(t, newHub(t, finished(read)))
	if got := attemptOf(t, snap, attemptID).Coverage.Gaps; got != 1 {
		t.Errorf("%d gaps, want the source's only", got)
	}
}

func TestGapsBeforeOneLineAreNumbered(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("gaps")
	read := script(origin, kube.EndComplete, streamtest.Line(1, "a"),
		gapStep{2, contract.GapReadInterrupted}, gapStep{2, contract.GapReadInterrupted}, streamtest.Line(2, "b"))
	snap := settled(t, newHub(t, finished(read)))
	want := []string{
		contract.EventID(attemptID, origin, 1, 0),
		contract.GapEventID(attemptID, origin, 2, 0),
		contract.GapEventID(attemptID, origin, 2, 1),
		contract.EventID(attemptID, origin, 2, 0),
	}
	if got := ids(snap.Events); !slices.Equal(got, want) {
		t.Errorf("events %q, want %q", got, want)
	}
}

func TestOutputsBlockWithoutEndIsDeliveredWhenTheLogEnds(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("outputs")
	read := script(origin, kube.EndComplete, streamtest.Line(1, "---KELOS_OUTPUTS_START---"), streamtest.Line(2, "branch: main"))
	snap := settled(t, newHub(t, finished(read)))
	if len(snap.Events) != 1 || snap.Events[0].Lifecycle == nil || snap.Events[0].Lifecycle.Phase != contract.LifecycleKelosOutputs {
		t.Errorf("events %+v, want the outputs block", snap.Events)
	}
}

func TestAttemptNoLongerFoundIsExpired(t *testing.T) {
	t.Parallel()
	gone := func(context.Context, kube.LogSink) (kube.ReadResult, error) {
		return kube.ReadResult{}, fmt.Errorf("%w: restarted", kube.ErrAttemptNotFound)
	}
	snap := settled(t, newHub(t, finished(gone)))
	if coverage := attemptOf(t, snap, attemptID).Coverage; coverage.State != contract.CoverageExpired || coverage.Reason != ReasonAttemptGone {
		t.Errorf("coverage %+v, want expired", coverage)
	}
}

func TestReadErrorIsReadAgain(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("error")
	failing := func(context.Context, kube.LogSink) (kube.ReadResult, error) {
		return kube.ReadResult{}, errors.New("connection refused")
	}
	snap := settled(t, newHub(t, finished(failing, complete(origin, 1, 2))))
	if got := len(snap.Events); got != 2 {
		t.Errorf("%d events, want the second read's 2", got)
	}
}

func TestEveryFixtureStreamsValidEvents(t *testing.T) {
	t.Parallel()
	for _, fx := range fixtureLogs(t) {
		t.Run(fx.file, func(t *testing.T) {
			t.Parallel()
			src := streamtest.NewSource(contract.PhaseSucceeded, fx.runner)
			src.SetAttempts(terminated())
			src.AddReads(attemptID, streamtest.Deliver(streamtest.Origin(fx.file), fx.lines, kube.EndComplete, ""))
			snap := settled(t, newHub(t, src))
			if len(snap.Events) == 0 {
				t.Fatal("no event")
			}
			for i, ev := range snap.Events {
				if ev.Seq <= 0 || (i > 0 && ev.Seq <= snap.Events[i-1].Seq) {
					t.Fatalf("event %d has seq %d after %d", i, ev.Seq, snap.Events[max(i-1, 0)].Seq)
				}
			}
		})
	}
}
