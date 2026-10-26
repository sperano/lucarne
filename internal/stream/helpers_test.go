package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

const (
	// testTimeout bounds every wait of a test.
	testTimeout = 5 * time.Second
	// testPoll is the poll interval and the step of eventually.
	testPoll = 2 * time.Millisecond
)

// attemptID is the fabricated run's first attempt.
var attemptID = streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerRunning).ID

// documents maps each SSE event name to the definition its data is.
var documents = map[string]string{
	contract.SSETranscript: contract.DefEvent,
	contract.SSEAttempt:    contract.DefAttemptUpdate,
	contract.SSERun:        contract.DefRunUpdate,
	contract.SSEEnd:        contract.DefEnd,
	contract.SSEReset:      contract.DefReset,
	contract.SSELive:       contract.DefLive,
}

// newHub is a hub reading src, polling fast; options change its config.
func newHub(t *testing.T, src Source, options ...func(*Config)) *Hub {
	t.Helper()
	cfg := Config{
		Source:        src,
		Logger:        slog.New(slog.DiscardHandler),
		PollInterval:  testPoll,
		RereadDelay:   time.Millisecond,
		IdleTimeout:   time.Hour,
		SettleTimeout: testTimeout,
	}
	for _, o := range options {
		o(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

// source is a source of the fabricated run in phase with attempts listed,
// whose lines the stream keeps raw.
func source(phase contract.RunPhase, attempts ...contract.Attempt) *streamtest.Source {
	src := streamtest.NewSource(phase, contract.RunnerUnknown)
	src.SetAttempts(attempts...)
	return src
}

func snapshotOf(t *testing.T, h *Hub) contract.Snapshot {
	t.Helper()
	data, err := h.Snapshot(context.Background(), streamtest.Key)
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(contract.DefSnapshot, data); err != nil {
		t.Fatal(err)
	}
	return decode[contract.Snapshot](t, data)
}

// cursorOf is the cursor that follows snap.
func cursorOf(snap contract.Snapshot) string {
	return Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq}.String()
}

func subscribe(t *testing.T, h *Hub, cursor string) *Subscription {
	t.Helper()
	sub, err := h.Subscribe(context.Background(), streamtest.Key, cursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	for _, u := range sub.Replay {
		check(t, u)
	}
	return sub
}

// next is sub's next update, checked against the schema.
func next(t *testing.T, sub *Subscription) *Update {
	t.Helper()
	select {
	case u := <-sub.Updates():
		check(t, u)
		return u
	case <-sub.Done():
		t.Fatalf("stream done, reset %v", sub.Err())
	case <-time.After(testTimeout):
		t.Fatal("no update")
	}
	return nil
}

// until reads sub's updates up to the first of event.
func until(t *testing.T, sub *Subscription, event string) []*Update {
	t.Helper()
	var updates []*Update
	for {
		u := next(t, sub)
		updates = append(updates, u)
		if u.Event == event {
			return updates
		}
	}
}

// check validates u's data against its definition, and its id.
func check(t *testing.T, u *Update) {
	t.Helper()
	def, ok := documents[u.Event]
	if !ok {
		t.Fatalf("unknown event %q", u.Event)
	}
	if err := contract.Validate(def, u.Data); err != nil {
		t.Fatalf("%s: %v", u.Event, err)
	}
	cursor, ok := ParseCursor(u.ID)
	numbered := u.Event != contract.SSEReset && u.Event != contract.SSELive
	if numbered != ok || (ok && cursor.Seq != u.seq) {
		t.Fatalf("%s has id %q, seq %d", u.Event, u.ID, u.seq)
	}
}

// decode decodes data into a T, with no unknown field.
func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

// events are the transcript events among updates.
func events(t *testing.T, updates []*Update) []contract.Event {
	t.Helper()
	var evs []contract.Event
	for _, u := range updates {
		if u.Event == contract.SSETranscript {
			evs = append(evs, decode[contract.Event](t, u.Data))
		}
	}
	return evs
}

// eventually waits until cond holds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(testPoll)
	}
}

// settled opens the run, waits until its stream ended and returns its
// snapshot.
func settled(t *testing.T, h *Hub) contract.Snapshot {
	t.Helper()
	snapshotOf(t, h)
	eventually(t, "the run to end", func() bool { return h.ended(streamtest.Key) })
	return snapshotOf(t, h)
}

// ended reports whether the run with key published its end.
func (h *Hub) ended(key contract.RunKey) bool {
	r := h.followed(key)
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ended != nil
}

// followed is the run with key the hub follows, nil for none.
func (h *Hub) followed(key contract.RunKey) *run {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.runs[key]; ok && isReady(e) {
		return e.run
	}
	return nil
}

func (r *run) subscribers() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subs)
}

// texts are the raw texts of evs, which a runner without adapter keeps.
func texts(evs []contract.Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Raw != nil {
			out = append(out, ev.Raw.Text)
		}
	}
	return out
}

// ids are the IDs of evs.
func ids(evs []contract.Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.ID
	}
	return out
}

// attemptOf is the attempt with id in snap.
func attemptOf(t *testing.T, snap contract.Snapshot, id string) contract.Attempt {
	t.Helper()
	for _, a := range snap.Attempts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no attempt %s in %+v", id, snap.Attempts)
	return contract.Attempt{}
}

// terminated is the fabricated run's first attempt, its container done.
func terminated() contract.Attempt {
	return streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerTerminated)
}

// running is the fabricated run's first attempt, its container running.
func running() contract.Attempt {
	return streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerRunning)
}

// complete is a read of lines from origin to the end of the log.
func complete(origin string, from, count int64) streamtest.Read {
	return streamtest.Deliver(origin, streamtest.Lines(from, count), kube.EndComplete, "")
}

// broken is a read of lines from origin that could not go on.
func broken(origin string, from, count int64) streamtest.Read {
	return streamtest.Deliver(origin, streamtest.Lines(from, count), kube.EndUnavailable, kube.ReasonSourceUnavailable)
}

// fixtureLog is a runner fixture as a log source delivers it.
type fixtureLog struct {
	file   string
	runner contract.RunnerKind
	lines  []adapter.Line
}

// fixtureLogs are the lines of every fixture of a runner with adapter.
func fixtureLogs(t *testing.T) []fixtureLog {
	t.Helper()
	manifest, err := fixtures.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	var logs []fixtureLog
	for _, fx := range manifest.Fixtures {
		data, err := fixtures.Read(fx.File)
		if err != nil {
			t.Fatal(err)
		}
		texts := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		lines := make([]adapter.Line, len(texts))
		for i, text := range texts {
			lines[i] = streamtest.Line(int64(i+1), text)
		}
		logs = append(logs, fixtureLog{file: fx.File, runner: contract.RunnerKind(fx.Runner), lines: lines})
	}
	if len(logs) == 0 {
		t.Fatal("no fixture")
	}
	return logs
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
