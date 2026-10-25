package claude

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/privacy"
)

var update = flag.Bool("update", false, "rewrite the golden files from the adapter's output")

// testAttempt is the attempt every test log belongs to.
const testAttempt = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0"

// runnerName is the runner the fixtures under test belong to.
const runnerName = "claude-code"

// normalize runs lines through a Normalizer with a Parser, numbers the
// events as a stream would and checks each against the schema.
func normalize(t *testing.T, lines ...string) []contract.Event {
	t.Helper()
	var first []byte
	if len(lines) > 0 {
		first = []byte(lines[0])
	}
	n := adapter.New(testAttempt, contract.LogOrigin(first), New())
	var events []contract.Event
	for i, line := range lines {
		events = append(events, n.Line(adapter.Line{Number: int64(i + 1), Text: []byte(line), Bytes: int64(len(line))})...)
	}
	events = append(events, n.End()...)
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	validate(t, events)
	return events
}

// validate checks every event against the schema and that no two share
// an ID.
func validate(t *testing.T, events []contract.Event) {
	t.Helper()
	ids := map[string]bool{}
	for _, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal %s: %v", ev.ID, err)
		}
		if err := contract.Validate(contract.DefEvent, data); err != nil {
			t.Errorf("event %s: %v\n%s", ev.ID, err, data)
		}
		if ids[ev.ID] {
			t.Errorf("event ID %s is not unique", ev.ID)
		}
		ids[ev.ID] = true
	}
}

// only returns the single event of kind, failing otherwise.
func only(t *testing.T, events []contract.Event, kind contract.EventKind) contract.Event {
	t.Helper()
	found := ofKind(events, kind)
	if len(found) != 1 {
		t.Fatalf("got %d %s events, want 1: %s", len(found), kind, dump(events))
	}
	return found[0]
}

func ofKind(events []contract.Event, kind contract.EventKind) []contract.Event {
	var found []contract.Event
	for _, ev := range events {
		if ev.Kind == kind {
			found = append(found, ev)
		}
	}
	return found
}

func dump(events []contract.Event) string {
	data, _ := json.MarshalIndent(events, "", "  ")
	return string(data)
}

func claudeFixtures(t *testing.T) []fixtures.Fixture {
	t.Helper()
	m, err := fixtures.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	var found []fixtures.Fixture
	for _, f := range m.Fixtures {
		if f.Runner == runnerName {
			found = append(found, f)
		}
	}
	if len(found) == 0 {
		t.Fatal("the manifest lists no Claude Code fixture")
	}
	return found
}

func fixtureLines(t *testing.T, file string) []string {
	t.Helper()
	data, err := fixtures.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestFixturesMatchGolden(t *testing.T) {
	t.Parallel()
	for _, f := range claudeFixtures(t) {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			lines := fixtureLines(t, f.File)
			events := normalize(t, lines...)
			checkEveryLineShown(t, lines, events)
			got, err := json.MarshalIndent(events, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			for _, finding := range privacy.Scan(string(got)) {
				t.Errorf("golden output: %v", finding)
			}
			golden := filepath.Join("testdata", strings.TrimSuffix(path.Base(f.File), ".log")+".golden.json")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s (go test -update rewrites it):\n%s", golden, got)
			}
		})
	}
}

// outputsLine matches the lines of a Kelos outputs block after its start.
var outputsLine = regexp.MustCompile(`^([a-z][a-z0-9-]*:( .*)?|---KELOS_OUTPUTS_END---)$`)

// checkEveryLineShown fails when a line that is not blank produced no
// event, unless it belongs to an outputs block shown as its start line's
// event.
func checkEveryLineShown(t *testing.T, lines []string, events []contract.Event) {
	t.Helper()
	shown := map[int64]contract.Event{}
	for _, ev := range events {
		shown[ev.Source.Line] = ev
	}
	inBlock := false
	for i, line := range lines {
		number := int64(i + 1)
		ev, ok := shown[number]
		switch {
		case ok:
			inBlock = ev.Kind == contract.KindLifecycle && ev.Lifecycle.Phase == contract.LifecycleKelosOutputs
		case line == "":
		case inBlock && outputsLine.MatchString(line):
		default:
			t.Errorf("line %d produced no event: %q", number, line)
		}
	}
}

func TestLineTypesAndRawReasons(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want contract.RawReason
	}{
		{"text", "(node:17) warning", contract.RawText},
		{"cut object", `{"type":"assistant","mess`, contract.RawMalformedJSON},
		{"array", `[1,2]`, contract.RawText},
		{"unknown type", `{"type":"example_future_type"}`, contract.RawUnknownType},
		{"no type", `{"example":1}`, contract.RawUnknownType},
		{"type not a string", `{"type":7}`, contract.RawUnknownType},
		{"unknown system subtype", `{"type":"system","subtype":"hook_started"}`, contract.RawUnknownType},
		{"rate limit", `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`, contract.RawUnsupported},
		{"partial message", `{"type":"stream_event","event":{"type":"message_start"}}`, contract.RawUnsupported},
		{"assistant without message", `{"type":"assistant"}`, contract.RawUnsupported},
		{"assistant with empty content", `{"type":"assistant","message":{"id":"msg_1","content":[]}}`, contract.RawUnsupported},
		{"user content not an array", `{"type":"user","message":{"content":7}}`, contract.RawUnsupported},
		{"init of the wrong shape", `{"type":"system","subtype":"init","model":7}`, contract.RawUnsupported},
		{"result of the wrong shape", `{"type":"result","is_error":"no"}`, contract.RawUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			raw := only(t, normalize(t, c.line), contract.KindRaw)
			if raw.Raw.Reason != c.want || raw.Raw.Text != c.line || raw.Raw.Bytes != int64(len(c.line)) {
				t.Errorf("raw = %+v, want reason %s holding the line", raw.Raw, c.want)
			}
		})
	}
}

func TestRawTextIsBounded(t *testing.T) {
	t.Parallel()
	line := `{"type":"example_future_type","data":"` + strings.Repeat("é", contract.MaxRawBytes) + `"}`
	raw := only(t, normalize(t, line), contract.KindRaw)
	if len(raw.Raw.Text) > contract.MaxRawBytes || raw.Raw.Bytes != int64(len(line)) {
		t.Errorf("raw text is %d bytes for a %d-byte line, bytes = %d", len(raw.Raw.Text), len(line), raw.Raw.Bytes)
	}
	if len(raw.Truncated) != 1 || raw.Truncated[0] != "text" {
		t.Errorf("truncated = %v, want [text]", raw.Truncated)
	}
}

func TestSystemLines(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"init","session_id":"s-1","model":"claude-opus-5-5","claude_code_version":"2.1.296"}`,
		`{"type":"system","subtype":"api_retry","attempt":0,"max_retries":10,"retry_delay_ms":5,"error_status":600,"error":"unknown"}`,
		`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual", "pre_tokens":5}}`,
		`{"type":"system","subtype":"compact_boundary"}`,
	)
	started := events[0].Lifecycle
	if started.Phase != contract.LifecycleSessionStarted || started.SessionID != "s-1" || started.RunnerVersion != "2.1.296" {
		t.Errorf("init = %+v", started)
	}
	retry := events[1].Error
	if retry.Retry != nil || retry.Status != nil || retry.Message != "unknown" {
		t.Errorf("api_retry with an invalid attempt and status = %+v, want neither", retry)
	}
	if d := events[2].Lifecycle.Detail; d == nil || *d != `{"trigger":"manual","pre_tokens":5}` {
		t.Errorf("compact detail = %v", d)
	}
	if events[3].Lifecycle.Phase != contract.LifecycleCompacted || events[3].Lifecycle.Detail != nil {
		t.Errorf("compact without metadata = %+v", events[3].Lifecycle)
	}
}

func TestRunnerTimeIsTheLineTimestamp(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","timestamp":"2026-10-25T05:00:07.25-04:00","message":{"id":"m","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}`,
		`{"type":"assistant","timestamp":"yesterday","message":{"id":"m","content":[{"type":"text","text":"c"}]}}`,
	)
	for _, ev := range events[:2] {
		if ev.Source.RunnerTime == nil || ev.Source.RunnerTime.Format("2006-01-02T15:04:05.000Z07:00") != "2026-10-25T09:00:07.250Z" {
			t.Errorf("runnerTime = %v, want the line's timestamp in UTC", ev.Source.RunnerTime)
		}
	}
	if events[2].Source.RunnerTime != nil {
		t.Errorf("an unparseable timestamp gave runnerTime %v", events[2].Source.RunnerTime)
	}
}

func FuzzLine(f *testing.F) {
	m, err := fixtures.LoadManifest()
	if err != nil {
		f.Fatal(err)
	}
	for _, fx := range m.Fixtures {
		if fx.Runner != runnerName {
			continue
		}
		data, err := fixtures.Read(fx.File)
		if err != nil {
			f.Fatal(err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			f.Add(line)
		}
	}
	f.Fuzz(func(t *testing.T, line string) {
		events := normalize(t, line, line)
		if line != "" && len(events) == 0 {
			t.Errorf("line %q produced no event", line)
		}
	})
}
