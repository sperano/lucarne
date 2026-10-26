package stream

import (
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

// live is a source of a running run whose attempt follows the lines sent
// on the channel returned.
func live(t *testing.T) (*streamtest.Source, chan<- string) {
	t.Helper()
	lines := make(chan string)
	src := source(contract.PhaseRunning, running())
	src.AddReads(attemptID, streamtest.Follow(streamtest.Origin("live"), lines))
	return src, lines
}

// send writes count lines numbered from on lines.
func send(lines chan<- string, from, count int) {
	for n := from; n < from+count; n++ {
		lines <- fmt.Sprintf(`{"n":%d}`, n)
	}
}

// waitEvents waits until the run's snapshot holds count events.
func waitEvents(t *testing.T, h *Hub, count int) contract.Snapshot {
	t.Helper()
	var snap contract.Snapshot
	eventually(t, strconv.Itoa(count)+" events", func() bool {
		snap = snapshotOf(t, h)
		return len(snap.Events) >= count
	})
	return snap
}

func TestSnapshotThenStreamHoldsEveryEventOnce(t *testing.T) {
	t.Parallel()
	const total, before = 300, 20
	src, lines := live(t)
	h := newHub(t, src)
	snapshotOf(t, h)
	go send(lines, 1, total)
	// The snapshot is taken while lines keep coming.
	snap := waitEvents(t, h, before)
	sub := subscribe(t, h, cursorOf(snap))
	if sub.Reset != nil {
		t.Fatalf("reset %s", sub.Reset.Data)
	}
	updates := slices.Clone(sub.Replay)
	held := len(snap.Events) + len(events(t, updates))
	for held < total {
		u := next(t, sub)
		updates = append(updates, u)
		if u.Event == contract.SSETranscript {
			held++
		}
	}
	for i, u := range updates {
		if want := snap.Stream.LastSeq + int64(i) + 1; u.seq != want {
			t.Fatalf("update %d has seq %d, want %d", i, u.seq, want)
		}
	}
	got := texts(append(slices.Clone(snap.Events), events(t, updates)...))
	for i, text := range got {
		if want := fmt.Sprintf(`{"n":%d}`, i+1); text != want {
			t.Fatalf("event %d is %s, want %s", i, text, want)
		}
	}
}

func TestReconnectResumesAfterTheLastEventID(t *testing.T) {
	t.Parallel()
	src, lines := live(t)
	h := newHub(t, src)
	first := subscribe(t, h, cursorOf(snapshotOf(t, h)))
	go send(lines, 1, 6)
	var got []*Update
	for len(events(t, got)) < 3 {
		got = append(got, next(t, first))
	}
	first.Close()
	if n := h.followed(streamtest.Key).subscribers(); n != 0 {
		t.Fatalf("%d subscribers after the client left, want 0", n)
	}
	second := subscribe(t, h, got[len(got)-1].ID)
	updates := slices.Clone(second.Replay)
	for len(events(t, updates)) < 3 {
		updates = append(updates, next(t, second))
	}
	if want := got[len(got)-1].seq + 1; updates[0].seq != want {
		t.Errorf("resumed at seq %d, want %d", updates[0].seq, want)
	}
	all := texts(append(events(t, got), events(t, updates)...))
	if want := []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`, `{"n":5}`, `{"n":6}`}; !slices.Equal(all, want) {
		t.Errorf("texts %q, want %q", all, want)
	}
}

func TestReplayIsFollowedByLive(t *testing.T) {
	t.Parallel()
	src, lines := live(t)
	h := newHub(t, src)
	snap := snapshotOf(t, h)
	send(lines, 1, 3)
	waitEvents(t, h, 3)
	sub := subscribe(t, h, cursorOf(snap))
	if len(sub.Replay) == 0 || sub.Live == nil {
		t.Fatalf("replay %d updates, live %v", len(sub.Replay), sub.Live)
	}
	check(t, sub.Live)
	marker := decode[contract.Live](t, sub.Live.Data)
	if last := sub.Replay[len(sub.Replay)-1].seq; marker.LastSeq != last {
		t.Errorf("live after seq %d, want the last replayed %d", marker.LastSeq, last)
	}
	go send(lines, 4, 1)
	if u := next(t, sub); u.seq != marker.LastSeq+1 {
		t.Errorf("first new update has seq %d, want %d", u.seq, marker.LastSeq+1)
	}
}

func TestCursorsThatCannotResumeReset(t *testing.T) {
	t.Parallel()
	src, lines := live(t)
	h := newHub(t, src, func(c *Config) { c.Bounds.ReplayUpdates = 2 })
	snap := snapshotOf(t, h)
	send(lines, 1, 5)
	// Once the attempt counts the lines, nothing more is published.
	eventually(t, "the attempt to count 5 lines", func() bool {
		return attemptOf(t, snapshotOf(t, h), attemptID).Coverage.Lines == 5
	})
	epoch := snap.Stream.Epoch
	tests := map[string]struct {
		cursor string
		reason contract.ResetReason
	}{
		"none":          {"", contract.ResetBadCursor},
		"no seq":        {epoch, contract.ResetBadCursor},
		"not canonical": {epoch + ":01", contract.ResetBadCursor},
		"negative":      {epoch + ":-1", contract.ResetBadCursor},
		"uppercase":     {"ABC:1", contract.ResetBadCursor},
		"in the future": {epoch + ":99", contract.ResetBadCursor},
		"other epoch":   {"0123abcd:1", contract.ResetEpochChanged},
		"evicted":       {epoch + ":1", contract.ResetReplayExpired},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			sub := subscribe(t, h, tt.cursor)
			if sub.Reset == nil {
				t.Fatalf("cursor %q resumed", tt.cursor)
			}
			check(t, sub.Reset)
			if reset := decode[contract.Reset](t, sub.Reset.Data); reset.Reason != tt.reason || reset.Epoch != epoch {
				t.Errorf("reset %+v, want %s in epoch %s", reset, tt.reason, epoch)
			}
		})
	}
	t.Run("still held", func(t *testing.T) {
		last := snapshotOf(t, h).Stream.LastSeq
		sub := subscribe(t, h, Cursor{Epoch: epoch, Seq: last - 2}.String())
		if sub.Reset != nil || len(sub.Replay) != 2 {
			t.Errorf("reset %v, replay %d updates, want 2", sub.Reset, len(sub.Replay))
		}
	})
}

func TestSlowClientIsResetAndReleased(t *testing.T) {
	t.Parallel()
	const queue = 2
	src, lines := live(t)
	h := newHub(t, src, func(c *Config) { c.Bounds.ClientQueue = queue })
	sub := subscribe(t, h, cursorOf(snapshotOf(t, h)))
	go send(lines, 1, queue*2)
	select {
	case <-sub.Done():
	case <-time.After(testTimeout):
		t.Fatal("a client that reads nothing was not reset")
	}
	check(t, sub.Err())
	if reset := decode[contract.Reset](t, sub.Err().Data); reset.Reason != contract.ResetClientTooSlow {
		t.Errorf("reset %+v, want client_too_slow", reset)
	}
	if n := h.followed(streamtest.Key).subscribers(); n != 0 {
		t.Errorf("%d subscribers, want the slow one released", n)
	}
	if len(sub.Updates()) != queue {
		t.Errorf("%d updates queued, want at most %d", len(sub.Updates()), queue)
	}
}

func TestRetainedEventsAreBounded(t *testing.T) {
	t.Parallel()
	const lines, kept = 5, 2
	read := func() streamtest.Read { return complete(streamtest.Origin("bounded"), 1, lines) }
	all := settled(t, newHub(t, finished(read()))).Events
	var newest int64
	for _, ev := range all[lines-kept:] {
		newest += int64(len(mustEncode(t, ev)))
	}
	tests := map[string]func(*Config){
		"by count": func(c *Config) { c.Bounds.EventsPerRun = kept },
		"by bytes": func(c *Config) { c.Bounds.RunBytes = newest },
	}
	for name, bound := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := settled(t, newHub(t, finished(read()), bound))
			if got, want := ids(snap.Events), ids(all[lines-kept:]); !slices.Equal(got, want) {
				t.Errorf("events %q, want the newest %q", got, want)
			}
			if snap.Stream.DroppedEvents != lines-kept {
				t.Errorf("%d dropped, want %d", snap.Stream.DroppedEvents, lines-kept)
			}
		})
	}
}

func TestStoppedRunEndsALateSubscription(t *testing.T) {
	t.Parallel()
	src, _ := live(t)
	h := newHub(t, src)
	snap := snapshotOf(t, h)
	r := h.followed(streamtest.Key)
	// The run is stopped between acquire and subscribe.
	r.stop()
	sub := r.subscribe(cursorOf(snap))
	t.Cleanup(sub.Close)
	select {
	case <-sub.Done():
	default:
		t.Fatal("a subscription to a stopped run stays open")
	}
	if sub.Err() != nil {
		t.Errorf("reset %s, want the stream ended without one", sub.Err().Data)
	}
	if n := r.subscribers(); n != 0 {
		t.Errorf("%d subscribers, want none", n)
	}
}
