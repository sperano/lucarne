package stream

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

// kinds are the event names of updates.
func kinds(updates []*Update) []string {
	out := make([]string, len(updates))
	for i, u := range updates {
		out[i] = u.Event
	}
	return out
}

func TestFinishedRunEndsItsStream(t *testing.T) {
	t.Parallel()
	h := newHub(t, finished(complete(streamtest.Origin("done"), 1, 3)))
	// The first snapshot of a finished run waits for its log.
	first := snapshotOf(t, h)
	if len(first.Events) != 3 {
		t.Errorf("first snapshot has %d events, want the log's 3", len(first.Events))
	}
	snap := settled(t, h)
	if coverage := attemptOf(t, snap, attemptID).Coverage; coverage.State != contract.CoverageEnded || coverage.Lines != 3 {
		t.Errorf("coverage %+v, want ended with 3 lines", coverage)
	}
	sub := subscribe(t, h, cursorOf(snap))
	if len(sub.Replay) != 1 || sub.Replay[0].Event != contract.SSEEnd {
		t.Fatalf("after the snapshot: %q, want the end alone", kinds(sub.Replay))
	}
	if end := decode[contract.End](t, sub.Replay[0].Data); end.Reason != contract.EndRunFinished || end.Seq != snap.Stream.LastSeq+1 {
		t.Errorf("end %+v, want run_finished after seq %d", end, snap.Stream.LastSeq)
	}
	if again := subscribe(t, h, sub.Replay[0].ID); !again.Finished {
		t.Errorf("a cursor at the end resumed: %+v", again)
	}
	after := Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq + 2}.String()
	if again := subscribe(t, h, after); !again.Finished {
		t.Errorf("a cursor after the end resumed: %+v", again)
	}
}

func TestRetryKeepsTheStreamOpen(t *testing.T) {
	t.Parallel()
	retry := streamtest.Attempt(streamtest.RetryPodUID, 0, contract.ContainerRunning)
	retryLines := make(chan string)
	src := source(contract.PhaseRunning, terminated())
	src.AddReads(attemptID, complete(streamtest.Origin("first"), 1, 1))
	src.AddReads(retry.ID, streamtest.Follow(streamtest.Origin("retry"), retryLines))
	h := newHub(t, src)
	var snap contract.Snapshot
	eventually(t, "the first attempt to end", func() bool {
		snap = snapshotOf(t, h)
		return attemptOf(t, snap, attemptID).Coverage.State == contract.CoverageEnded
	})
	sub := subscribe(t, h, cursorOf(snap))
	if len(sub.Replay) != 0 {
		t.Fatalf("a run whose retry has not started ended: %q", kinds(sub.Replay))
	}
	src.SetAttempts(terminated(), retry)
	src.SetPhase(contract.PhaseFailed)
	updates := until(t, sub, contract.SSERun)
	retryLines <- "retried"
	for len(events(t, updates)) == 0 || events(t, updates)[len(events(t, updates))-1].Attempt != retry.ID {
		updates = append(updates, next(t, sub))
	}
	if slices.Contains(kinds(updates), contract.SSEEnd) {
		t.Fatalf("stream ended while the retry ran: %q", kinds(updates))
	}
	close(retryLines)
	updates = until(t, sub, contract.SSEEnd)
	last := decode[contract.AttemptUpdate](t, updates[len(updates)-2].Data)
	if last.Attempt.ID != retry.ID || last.Attempt.Coverage.State != contract.CoverageEnded {
		t.Errorf("before the end: %+v, want the retry ended", last.Attempt)
	}
}

func TestDeletedRunExpiresItsAttemptsAndEnds(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"deleted":                     fmt.Errorf("%w: task gone", kube.ErrRunDeleted),
		"name reused by another task": fmt.Errorf("%w: task name now holds another task", kube.ErrRunDeleted),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, lines := live(t)
			h := newHub(t, src)
			sub := subscribe(t, h, cursorOf(snapshotOf(t, h)))
			send(lines, 1, 2)
			waitEvents(t, h, 2)
			src.SetRefreshError(err)
			updates := until(t, sub, contract.SSEEnd)
			tail := updates[len(updates)-4:]
			if want := []string{contract.SSERun, contract.SSETranscript, contract.SSEAttempt, contract.SSEEnd}; !slices.Equal(kinds(tail), want) {
				t.Fatalf("stream ended with %q, want %q", kinds(tail), want)
			}
			if run := decode[contract.RunUpdate](t, tail[0].Data).Run; !run.Deleted {
				t.Errorf("run %+v, want deleted", run)
			}
			gap := decode[contract.Event](t, tail[1].Data)
			if gap.ID != contract.GapEventID(attemptID, streamtest.Origin("live"), 3, 0) || gap.Gap.Reason != contract.GapAttemptExpired {
				t.Errorf("gap %+v, want attempt_expired before line 3", gap)
			}
			attempt := decode[contract.AttemptUpdate](t, tail[2].Data).Attempt
			if attempt.Coverage.State != contract.CoverageExpired || attempt.Coverage.Reason != kube.ReasonRunDeleted {
				t.Errorf("attempt %+v, want expired", attempt.Coverage)
			}
			if end := decode[contract.End](t, tail[3].Data); end.Reason != contract.EndRunDeleted {
				t.Errorf("end %+v, want run_deleted", end)
			}
		})
	}
}

func TestRestartIsAnotherAttempt(t *testing.T) {
	t.Parallel()
	restarted := streamtest.Attempt(streamtest.PodUID, 1, contract.ContainerRunning)
	src := source(contract.PhaseRunning, terminated())
	src.AddReads(attemptID, complete(streamtest.Origin("crashed"), 1, 1))
	src.AddReads(restarted.ID, complete(streamtest.Origin("restarted"), 1, 1))
	h := newHub(t, src)
	waitEvents(t, h, 1)
	src.SetAttempts(terminated(), restarted)
	snap := waitEvents(t, h, 2)
	if got := []string{snap.Events[0].Attempt, snap.Events[1].Attempt}; !slices.Equal(got, []string{attemptID, restarted.ID}) {
		t.Errorf("events of attempts %q, want the first, then the restart", got)
	}
	if len(snap.Attempts) != 2 {
		t.Errorf("attempts %+v, want both", snap.Attempts)
	}
}

func TestSourceFailureKeepsTheLastState(t *testing.T) {
	t.Parallel()
	src, lines := live(t)
	h := newHub(t, src)
	sub := subscribe(t, h, cursorOf(snapshotOf(t, h)))
	src.SetRefreshError(fmt.Errorf("%w: timeout", kube.ErrUnavailable))
	src.SetAttemptsError(errors.New("never asked"))
	send(lines, 1, 1)
	for u := next(t, sub); u.Event != contract.SSETranscript; u = next(t, sub) {
		if u.Event == contract.SSEEnd || u.Event == contract.SSERun {
			t.Fatalf("a failing source sent %s", u.Event)
		}
	}
	src.SetRefreshError(nil)
	src.SetAttemptsError(nil)
	src.SetPhase(contract.PhaseSucceeded)
	if run := decode[contract.RunUpdate](t, until(t, sub, contract.SSERun)[0].Data); run.Run.Phase != contract.PhaseSucceeded {
		t.Errorf("run %+v, want the new phase once the source answers", run.Run)
	}
}

func TestAttemptsAreBounded(t *testing.T) {
	t.Parallel()
	var attempts []contract.Attempt
	for restart := range int64(contract.MaxAttemptsPerRun + 2) {
		a := streamtest.Attempt(streamtest.PodUID, restart, contract.ContainerTerminated)
		a.Coverage = contract.Coverage{State: contract.CoverageExpired, Reason: kube.ReasonOlderRestart}
		attempts = append(attempts, a)
	}
	src := source(contract.PhaseRunning)
	h := newHub(t, src)
	snapshotOf(t, h)
	src.SetAttempts(attempts...)
	eventually(t, "the attempts to be listed", func() bool { return len(snapshotOf(t, h).Attempts) > 0 })
	snap := snapshotOf(t, h)
	if len(snap.Attempts) != contract.MaxAttemptsPerRun || snap.Attempts[0].Key.Restart != 2 {
		t.Errorf("%d attempts from restart %d, want the newest %d", len(snap.Attempts), snap.Attempts[0].Key.Restart, contract.MaxAttemptsPerRun)
	}
}
