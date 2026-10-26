package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

const (
	// minFragment is the shortest transcript text a log is searched for:
	// shorter ones would match unrelated log words.
	minFragment = 16
	// liveText starts every message the live tests write.
	liveText = "a line of the live transcript"
	// internalHost stands for what an error of the Kubernetes API can name.
	internalHost = "etcd-0.internal.example.com token=s3cr3t-0123456789"
)

// logBuffer collects the records of a hub and a server as JSON lines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// logger logs everything, debug records included, into the buffer.
func (b *logBuffer) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// textSet is the transcript texts a stack served: what its logs must
// never hold.
type textSet struct {
	mu        sync.Mutex
	fragments map[string]bool
}

// add keeps the first line of every message and raw text of events that
// is long enough to recognize.
func (s *textSet) add(events []contract.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fragments == nil {
		s.fragments = map[string]bool{}
	}
	for _, ev := range events {
		switch {
		case ev.Message != nil:
			s.keep(ev.Message.Text)
		case ev.Raw != nil:
			s.keep(ev.Raw.Text)
		}
	}
}

func (s *textSet) keep(text string) {
	line, _, _ := strings.Cut(text, "\n")
	if line = strings.TrimSpace(line); len(line) >= minFragment && utf8.ValidString(line) {
		s.fragments[line] = true
	}
}

func (s *textSet) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]string, 0, len(s.fragments))
	for f := range s.fragments {
		list = append(list, f)
	}
	return list
}

// transcriptFragments are pieces of transcript the world holds that no
// log may show: the hostile, private, foreign and long runs' content.
func transcriptFragments() []string {
	fragments := []string{
		scenario.ForeignMarker, scenario.PrivateMarker, scenario.HostileGlobal, liveText,
		"of a long session", "Hostile Markdown follows", "Done after the long output",
		"Lost with the pod", "The run's own line", "kept raw",
	}
	fragments = append(fragments, scenario.HostileMarkdown...)
	for _, raw := range scenario.HostileRaw {
		if utf8.ValidString(raw) && len(raw) >= minFragment && len(raw) < contract.MaxRawBytes {
			fragments = append(fragments, raw)
		}
	}
	return fragments
}

// leaks lists the records of the JSON lines in logs that hold a fragment,
// in a message, an attribute or anywhere in the line.
func leaks(logs []byte, fragments []string) []string {
	var found []string
	for i, line := range bytes.Split(logs, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		strs := []string{string(line)}
		if json.Unmarshal(line, &record) == nil {
			strs = append(strs, stringsIn(record)...)
		}
		if f := firstHeld(strs, fragments); f != "" {
			found = append(found, fmt.Sprintf("record %d holds %q", i, f))
		}
	}
	return found
}

// firstHeld is the first fragment some string holds, empty for none.
func firstHeld(strs, fragments []string) string {
	for _, f := range fragments {
		for _, s := range strs {
			if strings.Contains(s, f) {
				return f
			}
		}
	}
	return ""
}

// stringsIn lists every string a decoded JSON value holds.
func stringsIn(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case map[string]any:
		var all []string
		for _, e := range v {
			all = append(all, stringsIn(e)...)
		}
		return all
	case []any:
		var all []string
		for _, e := range v {
			all = append(all, stringsIn(e)...)
		}
		return all
	}
	return nil
}

// forbidden are the fragments no log of this stack may hold.
func (s *stack) forbidden() []string {
	return append(transcriptFragments(), s.texts.list()...)
}

// checkHygiene is what every test must keep true: the logs hold no
// transcript content, and the cluster was only read, in the allowlisted
// namespace only.
func (s *stack) checkHygiene() {
	s.t.Helper()
	for _, leak := range leaks(s.logs.bytes(), s.forbidden()) {
		s.t.Errorf("diagnostics: %s", leak)
	}
	if writes := s.world.Cluster.Writes(); len(writes) != 0 {
		s.t.Errorf("the source wrote to the cluster: %v", writes)
	}
	if s.world.Cluster.Namespaces()[scenario.PrivateNamespace] {
		s.t.Errorf("the source reached namespace %s, which is not allowlisted", scenario.PrivateNamespace)
	}
}

func TestLogCheckFlagsTranscript(t *testing.T) {
	logs := &logBuffer{}
	logger := logs.logger()
	logger.Warn("read log", "error", "boom")
	if got := leaks(logs.bytes(), transcriptFragments()); len(got) != 0 {
		t.Fatalf("a record without transcript is flagged: %v", got)
	}
	logger.Warn("read log", "line", `{"text":"`+scenario.PrivateMarker+`"}`)
	logger.Info("note", "nested", map[string]any{"deep": []string{"prefix " + scenario.HostileMarkdown[0]}})
	if got := leaks(logs.bytes(), transcriptFragments()); len(got) != 2 {
		t.Errorf("flagged %v, want the two records that hold transcript", got)
	}
}

func TestDiagnosticsHoldNoTranscript(t *testing.T) {
	st := newStack(t)
	w := st.world
	// A failing source first: the fake's reactors are not safe to add
	// while a run is being followed. A run the hub has not opened yet
	// meets the failure.
	late, err := w.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	restore := w.Cluster.Fail("list", "tasks", apierrors.NewForbidden(schema.GroupResource{Resource: "tasks"}, "", errors.New(internalHost)))
	st.apiError(st.get(late.Key.SnapshotPath()), http.StatusServiceUnavailable, contract.ErrorUnavailable)
	restore()

	readEveryKindOfRun(t, st)
	live, err := w.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	live.Pod.Write(scenario.ClaudeText(1, liveText+" one"), scenario.ClaudeText(2, liveText+" two"))
	live.Pod.Break()
	live.Pod.Write(scenario.ClaudeText(3, liveText+" three"))
	eventually(t, "the live lines to be read", func() bool {
		return len(st.snapshot(live.Key).Events) == 3
	})

	records := string(st.logs.bytes())
	if !strings.Contains(records, `"serve run"`) || !strings.Contains(records, "s3cr3t") {
		t.Errorf("the failing source was not logged (the logs are how it is diagnosed):\n%s", records)
	}
	if found := leaks(st.logs.bytes(), st.forbidden()); len(found) != 0 {
		t.Errorf("logs hold transcript: %v", found)
	}
	if len(st.texts.list()) == 0 {
		t.Error("no transcript text was collected: the check is vacuous")
	}
	st.assertNeverSent(internalHost, "s3cr3t")
}

// readEveryKindOfRun reads the runs that hold what no log may show: each
// fixture, the hostile, long, foreign and unknown runs, a run outside the
// allowlist, and a stream with a bad cursor.
func readEveryKindOfRun(t *testing.T, st *stack) {
	t.Helper()
	w := st.world
	files, err := scenario.FixtureFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		st.snapshot(w.Run(scenario.FixtureName(file)).Key)
	}
	for _, name := range []string{
		scenario.Hostile, scenario.LongLine, scenario.ForeignPod, scenario.PodDeleted, scenario.UnknownRunner,
	} {
		st.snapshot(w.Run(name).Key)
	}
	st.get(w.Run(scenario.PrivateRun).Key.SnapshotPath())
	st.get(w.Run(scenario.Hostile).Key.EventsPath() + "?after=nonsense")
}
