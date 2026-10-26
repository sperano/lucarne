package e2e

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/stream"
)

func TestServerBusyWhenEveryRunIsWatched(t *testing.T) {
	const maxRuns = 2
	st := newStack(t, func(c *stream.Config) { c.MaxRuns = maxRuns })
	var watched []*liveFixture
	for i := 0; i < maxRuns; i++ {
		watched = append(watched, newLive(t, st))
	}
	third, err := st.world.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, path := range []string{third.Key.SnapshotPath(), third.Key.EventsPath()} {
		r := st.get(path)
		body := st.apiError(r, http.StatusServiceUnavailable, contract.ErrorUnavailable)
		checkJSONHeaders(t, r)
		messages = append(messages, body.Message)
	}
	if messages[0] != messages[1] || messages[0] == "" {
		t.Errorf("busy messages %q: want one fixed message", messages)
	}
	// Any run the hub does not follow is turned away, a finished one too;
	// the runs it follows are served as before.
	unfollowed := st.world.Run(scenario.Hostile)
	st.apiError(st.get(unfollowed.Key.SnapshotPath()), http.StatusServiceUnavailable, contract.ErrorUnavailable)
	for _, w := range watched {
		st.snapshot(w.run.Key)
	}
}

// writePadded appends one message of size more bytes than its number.
func (l *liveFixture) writePadded(size int) {
	l.written++
	l.run.Pod.Write(scenario.ClaudeText(l.written, liveMessage(l.written)+" "+strings.Repeat("p", size)))
}

func TestSlowClientGetsClientTooSlowReset(t *testing.T) {
	const queue, lines, padding = 4, 40, 8 << 10
	st := newStackWith(t, true, func(c *stream.Config) { c.Bounds.ClientQueue = queue })
	l := newLive(t, st)
	fast := l.events
	snap := st.snapshot(l.run.Key)
	if snap.Limits.MaxClientQueue != queue {
		t.Errorf("snapshot publishes maxClientQueue %d, want the hub's %d", snap.Limits.MaxClientQueue, queue)
	}
	// This client's reads stop once its first frames are buffered.
	slow := st.events(l.run.Key, streamOptions{after: cursor(snap), client: st.slowClient()})
	for i := 0; i < lines; i++ {
		l.writePadded(padding)
		// The fast client keeps up one line at a time, so only the slow
		// one can fall behind.
		fast.transcript(1)
	}

	frames := slow.drain()
	last := frames[len(frames)-1]
	if last.Event != contract.SSEReset || last.reset(t).Reason != contract.ResetClientTooSlow {
		t.Fatalf("the slow client's stream ends with %s %s, want a client_too_slow reset: %v", last.Event, last.Data, summary(frames))
	}
	if got := last.reset(t).Epoch; got != snap.Stream.Epoch {
		t.Errorf("reset epoch %s, want %s", got, snap.Stream.Epoch)
	}
	// What it got before the reset is a gapless prefix (checked as read);
	// the reset says the rest was dropped.
	if delivered := len(ofEvent(frames, contract.SSETranscript)); delivered >= lines {
		t.Errorf("the slow client got all %d events: nothing was dropped", delivered)
	}
	// Recovery is a new snapshot, whole, and the fast client lost nothing.
	whole := st.snapshot(l.run.Key)
	if len(whole.Events) != lines {
		t.Errorf("snapshot holds %d events, want %d", len(whole.Events), lines)
	}
}

func TestLongSessionRetentionAndReplayAreBounded(t *testing.T) {
	st := newStack(t)
	key := st.world.Run(scenario.LongSession).Key
	start := time.Now()
	snap, r := st.hugeSnapshot(key)
	t.Logf("snapshot of %d events: %d bytes in %s", len(snap.Events), len(r.Body), time.Since(start))

	const lines = scenario.LongSessionMessages + 1 // the init line
	if got := len(snap.Events); got != contract.MaxEventsPerRun {
		t.Fatalf("%d events retained, want exactly the limit %d", got, contract.MaxEventsPerRun)
	}
	if want := int64(lines - contract.MaxEventsPerRun); snap.Stream.DroppedEvents != want {
		t.Errorf("droppedEvents %d, want %d: one event a line, the oldest dropped", snap.Stream.DroppedEvents, want)
	}
	if c := snap.Attempts[0].Coverage; c.State != contract.CoverageEnded || c.Lines != lines || c.Gaps != 0 {
		t.Errorf("coverage %+v: the whole log was read", c)
	}
	if snap.Limits.MaxEventsPerRun != contract.MaxEventsPerRun || snap.Limits.MaxReplayUpdates != contract.MaxReplayUpdates {
		t.Errorf("limits %+v do not publish the bounds", snap.Limits)
	}
	// The oldest are dropped: the newest remains.
	first, last := snap.Events[0], snap.Events[len(snap.Events)-1]
	wantFirst := scenario.LongSessionMessages - contract.MaxEventsPerRun + 1 // line 1 is the init
	if first.Message == nil || !strings.HasPrefix(first.Message.Text, "Message "+strconv.Itoa(wantFirst)+" ") {
		t.Errorf("first retained event %s, want message %d", kindsOf(snap.Events[:1]), wantFirst)
	}
	if last.Message == nil || !strings.HasPrefix(last.Message.Text, "Message "+strconv.Itoa(scenario.LongSessionMessages)+" ") {
		t.Errorf("last retained event %s, want the last message", kindsOf(snap.Events[len(snap.Events)-1:]))
	}
	checkEventsOrdered(t, snap, snap.Attempts[0].ID)

	// Replay reaches back MaxReplayUpdates updates and no further.
	epoch := snap.Stream.Epoch
	within := stream.Cursor{Epoch: epoch, Seq: snap.Stream.LastSeq - contract.MaxReplayUpdates + 1}
	frames := st.events(key, streamOptions{after: within.String(), seqOnly: true}).drain()
	if len(frames) != contract.MaxReplayUpdates || frames[len(frames)-1].Event != contract.SSEEnd {
		t.Errorf("replay from the oldest cursor held: %d frames, want %d ending with end", len(frames), contract.MaxReplayUpdates)
	}
	expired := stream.Cursor{Epoch: epoch, Seq: snap.Stream.LastSeq - contract.MaxReplayUpdates - 1}
	got := st.events(key, streamOptions{after: expired.String()}).drain()
	if len(got) != 1 || got[0].Event != contract.SSEReset || got[0].reset(t).Reason != contract.ResetReplayExpired {
		t.Errorf("a cursor older than the replay: %v, want one replay_expired reset", summary(got))
	}
}

func TestSmallBoundsAreEnforcedAndPublished(t *testing.T) {
	const events, replay = 5, 3
	st := newStack(t, func(c *stream.Config) {
		c.Bounds = stream.Bounds{EventsPerRun: events, ReplayUpdates: replay}
	})
	key := st.world.Run(scenario.FixtureName("claude-code/2.1.296/session-basic.log")).Key
	snap := st.snapshot(key)
	if len(snap.Events) != events || snap.Limits.MaxEventsPerRun != events || snap.Limits.MaxReplayUpdates != replay {
		t.Fatalf("%d events, limits %+v: want %d events and the bounds published", len(snap.Events), snap.Limits, events)
	}
	lines := snap.Attempts[0].Coverage.Lines
	if snap.Stream.DroppedEvents < 1 || int64(events)+snap.Stream.DroppedEvents > lines {
		t.Errorf("droppedEvents %d with %d retained of %d lines", snap.Stream.DroppedEvents, events, lines)
	}
	// The retained events are the newest.
	all := newStack(t).snapshot(key)
	for i, ev := range snap.Events {
		if want := all.Events[len(all.Events)-events+i]; ev.Source.Line != want.Source.Line {
			t.Errorf("retained event %d is line %d, want the newest: line %d", i, ev.Source.Line, want.Source.Line)
		}
	}
	// A cursor within the replay resumes; an older one is expired.
	within := stream.Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq - replay + 1}
	if got := st.events(key, streamOptions{after: within.String()}).drain(); len(got) != replay {
		t.Errorf("replay of %d updates: %v", replay, summary(got))
	}
	old := stream.Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq - replay - 1}
	got := st.events(key, streamOptions{after: old.String()}).drain()
	if len(got) != 1 || got[0].reset(t).Reason != contract.ResetReplayExpired {
		t.Errorf("an expired cursor: %v", summary(got))
	}
}

func TestOversizedOutputIsCutAndMarked(t *testing.T) {
	st := newStack(t)
	snap := st.snapshot(st.world.Run(scenario.LongLine).Key)
	var oversized, longText, longOutput *contract.Event
	for i := range snap.Events {
		ev := &snap.Events[i]
		switch {
		case ev.Raw != nil && ev.Raw.Reason == contract.RawOversized:
			oversized = ev
		case ev.Message != nil && len(ev.Truncated) > 0:
			longText = ev
		case ev.ToolResult != nil && len(ev.Truncated) > 0:
			longOutput = ev
		}
	}
	if oversized == nil || longText == nil || longOutput == nil {
		t.Fatalf("events %s: want an oversized raw event and a truncated text and tool output", kindsOf(snap.Events))
	}
	if len(oversized.Raw.Text) > contract.MaxRawBytes || oversized.Raw.Bytes <= contract.MaxLineBytes || !utf8.ValidString(oversized.Raw.Text) {
		t.Errorf("oversized raw event: %d bytes of text for a line of %d", len(oversized.Raw.Text), oversized.Raw.Bytes)
	}
	if len(longText.Message.Text) > contract.MaxTextBytes || strings.Join(longText.Truncated, ",") != "text" {
		t.Errorf("long message: %d bytes, truncated %v", len(longText.Message.Text), longText.Truncated)
	}
	if out := longOutput.ToolResult.Output; out == nil || len(*out) > contract.MaxToolOutputBytes || strings.Join(longOutput.Truncated, ",") != "output" {
		t.Errorf("long tool output: truncated %v", longOutput.Truncated)
	}
	c := snap.Attempts[0].Coverage
	if c.TruncatedLines != 1 || c.State != contract.CoverageEnded {
		t.Errorf("coverage %+v: the oversized line is counted as truncated", c)
	}
	// The log goes on after the long output.
	if last := snap.Events[len(snap.Events)-1]; last.Message == nil || !strings.Contains(last.Message.Text, "Done after") {
		t.Errorf("the last event is %s, want the message after the long output", kindsOf(snap.Events[len(snap.Events)-1:]))
	}
}
