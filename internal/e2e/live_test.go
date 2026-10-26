package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/kube/kubetest"
)

// liveFixture is a live run being followed over HTTP.
type liveFixture struct {
	st      *stack
	run     *scenario.Run
	events  *sseClient
	written int
}

// newLive adds a running run, loads its snapshot and follows its stream
// from there up to the live marker.
func newLive(t *testing.T, st *stack) *liveFixture {
	t.Helper()
	run, err := st.world.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	snap := st.snapshot(run.Key)
	if snap.Run.Phase != contract.PhaseRunning || len(snap.Attempts) != 1 || len(snap.Events) != 0 {
		t.Fatalf("a new live run: phase %s, %d attempts, %d events", snap.Run.Phase, len(snap.Attempts), len(snap.Events))
	}
	l := &liveFixture{st: st, run: run, events: st.events(run.Key, streamOptions{after: cursor(snap)})}
	l.events.untilLive()
	return l
}

// write appends n messages to the run's log, numbered after the last.
func (l *liveFixture) write(n int) {
	lines := make([]string, n)
	for i := range lines {
		l.written++
		lines[i] = scenario.ClaudeText(l.written, liveMessage(l.written))
	}
	l.run.Pod.Write(lines...)
}

func liveMessage(n int) string { return fmt.Sprintf("%s %d", liveText, n) }

// wantMessages checks that events are the messages from..to, in order.
func wantMessages(t *testing.T, events []contract.Event, from, to int) {
	t.Helper()
	if len(events) != to-from+1 {
		t.Fatalf("%d events, want messages %d to %d: %s", len(events), from, to, kindsOf(events))
	}
	for i, ev := range events {
		want := liveMessage(from + i)
		if ev.Message == nil || ev.Message.Text != want {
			t.Fatalf("event %d is %s, want the message %q", i, kindsOf(events[i:i+1]), want)
		}
	}
}

func kindsOf(events []contract.Event) string {
	s := ""
	for _, ev := range events {
		s += string(ev.Kind)
		if ev.Gap != nil {
			s += "(" + string(ev.Gap.Reason) + ")"
		}
		s += " "
	}
	return s
}

func TestLiveStreamDeliversWritesInOrder(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	const batches, perBatch = 4, 5
	for b := 0; b < batches; b++ {
		l.write(perBatch)
		wantMessages(t, l.events.transcript(perBatch), b*perBatch+1, (b+1)*perBatch)
	}
	snap := st.coverageEventually(l.run.Key, "all lines counted", func(c contract.Coverage) bool {
		return c.Lines == batches*perBatch
	})
	wantMessages(t, snap.Events, 1, batches*perBatch)
	if c := snap.Attempts[0].Coverage; c.State != contract.CoverageFollowing || c.Gaps != 0 {
		t.Errorf("coverage %+v, want following with no gap", c)
	}
}

// coverageEventually is the snapshot of a run once its attempt's coverage
// satisfies done: the attempt's counts are published as the source polls,
// a moment after the events they count.
func (s *stack) coverageEventually(key contract.RunKey, what string, done func(contract.Coverage) bool) contract.Snapshot {
	s.t.Helper()
	var snap contract.Snapshot
	eventually(s.t, what, func() bool {
		snap = s.snapshot(key)
		return len(snap.Attempts) == 1 && done(snap.Attempts[0].Coverage)
	})
	return snap
}

func TestLiveBreaksLoseAndRepeatNothing(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	const batches, perBatch = 8, 7
	// Each batch is read before the next break, so that every break meets
	// a stream with lines already delivered, to line up the next read on.
	for b := 0; b < batches; b++ {
		l.write(perBatch)
		wantMessages(t, l.events.transcript(perBatch), b*perBatch+1, (b+1)*perBatch)
		l.run.Pod.Break()
	}
	// Then breaks while lines are being written.
	const streamed = 200
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < streamed; i++ {
			l.write(1)
			if i%perBatch == 0 {
				l.run.Pod.Break()
			}
		}
	}()
	got := l.events.transcript(streamed)
	<-done
	wantMessages(t, got, batches*perBatch+1, batches*perBatch+streamed)

	total := batches*perBatch + streamed
	snap := st.coverageEventually(l.run.Key, "all lines counted", func(c contract.Coverage) bool {
		return c.Lines == int64(total)
	})
	wantMessages(t, snap.Events, 1, total)
	if c := snap.Attempts[0].Coverage; c.Gaps != 0 {
		t.Errorf("coverage %+v: a break the source can line up again is no gap", c)
	}
	if opens := podOpens(st, snap.Attempts[0].PodName); opens <= batches {
		t.Errorf("the log was opened %d times: the breaks were not followed by reads", opens)
	}
}

// podOpens is how many times the log of the pod was opened.
func podOpens(st *stack, pod string) int {
	n := 0
	for _, o := range st.world.Cluster.LogOpens() {
		if o.Pod == pod {
			n++
		}
	}
	return n
}

func TestLiveRotationIsAGapInStreamAndSnapshot(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	const before, after = 3, 3
	l.write(before)
	wantMessages(t, l.events.transcript(before), 1, before)

	// The kubelet rotated the log: what was read is gone, so the reader
	// cannot prove that no line was lost between it and the new log.
	l.run.Pod.Rotate()
	l.run.Pod.Break()
	l.write(after)
	got := l.events.transcript(after + 1)
	if got[0].Kind != contract.KindGap || got[0].Gap.Reason != contract.GapReadInterrupted {
		t.Fatalf("the stream shows %s, want a read_interrupted gap first", kindsOf(got))
	}
	wantMessages(t, got[1:], before+1, before+after)

	snap := st.coverageEventually(l.run.Key, "the gap counted", func(c contract.Coverage) bool {
		return c.Gaps == 1 && c.Lines == before+after
	})
	if len(snap.Events) != before+after+1 {
		t.Fatalf("snapshot: %s", kindsOf(snap.Events))
	}
	wantMessages(t, snap.Events[:before], 1, before)
	if gap := snap.Events[before]; gap.ID != got[0].ID || gap.Source.Line != before+1 {
		t.Errorf("snapshot gap %s at line %d, want %s before line %d", gap.ID, gap.Source.Line, got[0].ID, before+1)
	}
	wantMessages(t, snap.Events[before+1:], before+1, before+after)
	if c := snap.Attempts[0].Coverage; c.State != contract.CoverageFollowing {
		t.Errorf("coverage %+v, want following with one gap", c)
	}
}

func TestLiveFinishEndsTheStream(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	l.write(3)
	l.events.transcript(3)
	if err := l.run.Finish(0, kubetest.PhaseSucceeded); err != nil {
		t.Fatal(err)
	}
	frames := l.events.drain()
	if len(frames) == 0 || frames[len(frames)-1].Event != contract.SSEEnd {
		t.Fatalf("the stream did not end with the end event: %v", summary(frames))
	}
	// The run's last states come before the end.
	var sawSucceeded, sawEnded bool
	for _, f := range frames[:len(frames)-1] {
		sawSucceeded = sawSucceeded || (f.Event == contract.SSERun && strings.Contains(f.Data, `"phase":"succeeded"`))
		sawEnded = sawEnded || (f.Event == contract.SSEAttempt && strings.Contains(f.Data, `"state":"ended"`))
	}
	if !sawSucceeded || !sawEnded {
		t.Errorf("before the end: run succeeded %v, attempt ended %v: %v", sawSucceeded, sawEnded, summary(frames))
	}
	snap := st.snapshot(l.run.Key)
	if snap.Run.Phase != contract.PhaseSucceeded || snap.Attempts[0].Coverage.State != contract.CoverageEnded {
		t.Errorf("run %s, coverage %+v", snap.Run.Phase, snap.Attempts[0].Coverage)
	}
	// A client that loaded the snapshot of the ended run is still sent the
	// end, once; one that saw it is told to stay away.
	again := st.events(l.run.Key, streamOptions{after: cursor(snap)})
	replay := again.drain()
	if again.Status != http.StatusOK || len(replay) != 1 || replay[0].Event != contract.SSEEnd {
		t.Fatalf("stream from the snapshot: status %d, frames %v, want the end alone", again.Status, summary(replay))
	}
	if r := replay[0].end(t); r.Reason != contract.EndRunFinished {
		t.Errorf("end %+v, want run_finished", r)
	}
	if done := st.events(l.run.Key, streamOptions{lastEventID: replay[0].ID}); done.Status != http.StatusNoContent {
		t.Errorf("reconnect after the end: status %d, want 204", done.Status)
	}
}
