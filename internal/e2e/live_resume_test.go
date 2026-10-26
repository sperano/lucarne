package e2e

import (
	"net/http"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube/kubetest"
)

// numbered are the frames that carry an id: the updates a cursor counts.
func numbered(frames []frame) []frame {
	var found []frame
	for _, f := range frames {
		if f.ID != "" {
			found = append(found, f)
		}
	}
	return found
}

// after is the frames following the one with id, nil when there is none
// with it.
func after(frames []frame, id string) []frame {
	for i, f := range frames {
		if f.ID == id {
			return frames[i+1:]
		}
	}
	return nil
}

func sameFrames(t *testing.T, what string, got, want []frame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d frames, want %d\n got %v\nwant %v", what, len(got), len(want), summary(got), summary(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: frame %d is %s %s, want %s %s", what, i, got[i].Event, got[i].ID, want[i].Event, want[i].ID)
		}
	}
}

func TestLiveReconnectResumesExactlyAfterTheLastEventID(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	continuous := l.events
	const first, mid = 10, 4
	l.write(first)
	continuous.transcript(first)
	events := ofEvent(continuous.Seen, contract.SSETranscript)
	midID := events[mid-1].ID

	// A browser reconnecting sends Last-Event-ID; it wins over after.
	reconnects := map[string]*sseClient{
		"Last-Event-ID":            st.events(l.run.Key, streamOptions{lastEventID: midID}),
		"Last-Event-ID over after": st.events(l.run.Key, streamOptions{after: events[0].ID, lastEventID: midID}),
		"after":                    st.events(l.run.Key, streamOptions{after: midID}),
	}
	for name, c := range reconnects {
		checkReplay(t, name, c, after(numbered(continuous.Seen), midID))
	}

	const second = 6
	l.write(second)
	continuous.transcript(second)
	if err := l.run.Finish(0, kubetest.PhaseSucceeded); err != nil {
		t.Fatal(err)
	}
	continuous.drain()
	want := after(numbered(continuous.Seen), midID)
	for name, c := range reconnects {
		c.drain()
		sameFrames(t, name+" after the end", numbered(c.Seen), want)
	}
	if last := want[len(want)-1]; last.Event != contract.SSEEnd {
		t.Fatalf("the stream ends with %s, want end", last.Event)
	}
	checkSnapshotHolds(t, st.snapshot(l.run.Key), transcriptOf(t, numbered(continuous.Seen)))
}

// checkReplay reads the replay of a reconnected stream and compares its
// events with the ones a continuous subscriber saw after the cursor. The
// attempt updates around them come as the source polls, so only the
// events are compared before the end.
func checkReplay(t *testing.T, name string, c *sseClient, continuous []frame) {
	t.Helper()
	replay := c.untilLive()
	want := ofEvent(continuous, contract.SSETranscript)
	sameFrames(t, name+" replay", ofEvent(replay, contract.SSETranscript), want)
	var live contract.Live
	mustUnmarshal(t, []byte(replay[len(replay)-1].Data), &live)
	if lastEvent := seqOf(t, want[len(want)-1]); live.LastSeq < lastEvent {
		t.Errorf("%s: live marker at %d, before the last event replayed (%d)", name, live.LastSeq, lastEvent)
	}
}

// checkSnapshotHolds checks that a snapshot holds exactly the events a
// stream carried.
func checkSnapshotHolds(t *testing.T, snap contract.Snapshot, streamed []contract.Event) {
	t.Helper()
	if len(streamed) != len(snap.Events) {
		t.Fatalf("the stream carried %d events, the snapshot holds %d", len(streamed), len(snap.Events))
	}
	for i, ev := range streamed {
		if payloadJSON(t, ev) != payloadJSON(t, snap.Events[i]) {
			t.Fatalf("event %d: stream and snapshot differ", i+1)
		}
	}
}

func seqOf(t *testing.T, f frame) int64 {
	t.Helper()
	var u struct {
		Seq int64 `json:"seq"`
	}
	mustUnmarshal(t, []byte(f.Data), &u)
	return u.Seq
}

func TestLiveDeletedTaskIsAnExplicitEnd(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	const lines = 3
	l.write(lines)
	wantMessages(t, l.events.transcript(lines), 1, lines)

	// Kelos deletes a finished Task after its TTL; here it goes while
	// the stream is open.
	if err := l.run.Task.Delete(); err != nil {
		t.Fatal(err)
	}
	frames := l.events.drain()
	last := frames[len(frames)-1]
	if last.Event != contract.SSEEnd || last.end(t).Reason != contract.EndRunDeleted {
		t.Fatalf("the stream ends with %s %s, want end run_deleted", last.Event, last.Data)
	}
	var sawDeleted bool
	for _, f := range ofEvent(frames, contract.SSERun) {
		var u contract.RunUpdate
		mustUnmarshal(t, []byte(f.Data), &u)
		sawDeleted = sawDeleted || u.Run.Deleted
	}
	if !sawDeleted {
		t.Errorf("no run update says the run is deleted: %v", summary(frames))
	}

	// The hub still holds the run: its snapshot says what happened.
	checkDeletedSnapshot(t, st.snapshot(l.run.Key), lines)
	if again := st.events(l.run.Key, streamOptions{after: last.ID}); again.Status != http.StatusNoContent {
		t.Errorf("reconnect after the end: status %d, want 204", again.Status)
	}

	// A server that does not know the run cannot tell it expired from one
	// that never was.
	fresh := newStack(t)
	for _, path := range []string{l.run.Key.SnapshotPath(), l.run.Key.EventsPath()} {
		fresh.apiError(fresh.get(path), http.StatusNotFound, contract.ErrorRunNotFound)
	}
}

// checkDeletedSnapshot checks that the snapshot of a run whose Task was
// deleted says so: the run is marked, and the log is gone with the Task,
// which the attempt's coverage and a gap where the lines stop tell.
func checkDeletedSnapshot(t *testing.T, snap contract.Snapshot, lines int) {
	t.Helper()
	if !snap.Run.Deleted {
		t.Error("the snapshot of a deleted run does not say so")
	}
	wantMessages(t, snap.Events[:lines], 1, lines)
	cov := snap.Attempts[0].Coverage
	final := snap.Events[len(snap.Events)-1]
	switch {
	case cov.State == contract.CoverageExpired && cov.Reason != "" && final.Gap != nil && final.Gap.Reason == contract.GapAttemptExpired:
	case cov.State == contract.CoverageEnded && cov.Lines == int64(lines):
		// The reader saw the log end before the Task was seen gone.
	default:
		t.Errorf("coverage %+v, last event %s: want the log expired with an attempt_expired gap", cov, kindsOf(snap.Events[len(snap.Events)-1:]))
	}
}
