package codex

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
const testAttempt = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.codex.0"

// runnerName is the runner the fixtures under test belong to.
const runnerName = "codex"

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

// onLine returns the events line produced.
func onLine(events []contract.Event, line int64) []contract.Event {
	var found []contract.Event
	for _, ev := range events {
		if ev.Source.Line == line {
			found = append(found, ev)
		}
	}
	return found
}

func dump(events []contract.Event) string {
	data, _ := json.MarshalIndent(events, "", "  ")
	return string(data)
}

func codexFixtures(t *testing.T) []fixtures.Fixture {
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
		t.Fatal("the manifest lists no Codex fixture")
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
	for _, f := range codexFixtures(t) {
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

// TestFixturesRenderEachChangeOnce checks, over every fixture, that each
// result follows its call, that no call has two results, and that no
// message or tool call is rendered twice with the same content.
func TestFixturesRenderEachChangeOnce(t *testing.T) {
	t.Parallel()
	for _, f := range codexFixtures(t) {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			calls := map[string]bool{}
			results := map[string]bool{}
			rendered := map[string]bool{}
			for _, ev := range normalize(t, fixtureLines(t, f.File)...) {
				var key string
				switch ev.Kind {
				case contract.KindMessage:
					key = "message " + ev.Message.MessageID + " " + ev.Message.Text
				case contract.KindToolCall:
					calls[ev.ToolCall.CallID] = true
					key = "call " + ev.ToolCall.CallID + " " + string(ev.ToolCall.Input)
				case contract.KindToolResult:
					id := ev.ToolResult.CallID
					if !calls[id] || results[id] {
						t.Errorf("result %s: call shown %v, result shown before %v", ev.ID, calls[id], results[id])
					}
					results[id] = true
				}
				if key != "" && rendered[key] {
					t.Errorf("%s renders %q again", ev.ID, key)
				}
				rendered[key] = true
			}
		})
	}
}

// outputsLine matches the lines of a Kelos outputs block after its start.
var outputsLine = regexp.MustCompile(`^([a-z][a-z0-9-]*:( .*)?|---KELOS_OUTPUTS_END---)$`)

// checkEveryLineShown fails when a line that is not blank produced no
// event, unless it belongs to an outputs block shown as its start line's
// event, or is a later line of an item an earlier line showed: a repeat.
func checkEveryLineShown(t *testing.T, lines []string, events []contract.Event) {
	t.Helper()
	shown := map[int64]contract.Event{}
	for _, ev := range events {
		shown[ev.Source.Line] = ev
	}
	inBlock := false
	itemLines := map[string]bool{}
	for i, line := range lines {
		number := int64(i + 1)
		id, isItem := itemID(line)
		ev, ok := shown[number]
		switch {
		case ok:
			inBlock = ev.Kind == contract.KindLifecycle && ev.Lifecycle.Phase == contract.LifecycleKelosOutputs
		case line == "":
		case inBlock && outputsLine.MatchString(line):
		case isItem && itemLines[id]:
		default:
			t.Errorf("line %d produced no event: %q", number, line)
		}
		if isItem {
			itemLines[id] = true
		}
	}
}

// itemID is the ID of an item line's item.
func itemID(line string) (string, bool) {
	var l struct {
		Item json.RawMessage `json:"item"`
	}
	if json.Unmarshal([]byte(line), &l) != nil || l.Item == nil {
		return "", false
	}
	id, ok := firstID(l.Item)
	return id, ok && id != ""
}

func TestLineTypesAndRawReasons(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want contract.RawReason
	}{
		{"text", "Warning: example notice", contract.RawText},
		{"cut object", `{"type":"item.completed","item":{"id":"item_1","te`, contract.RawMalformedJSON},
		{"array", `[1,2]`, contract.RawText},
		{"unknown type", `{"type":"thread.example_future_event"}`, contract.RawUnknownType},
		{"no type", `{"example":1}`, contract.RawUnknownType},
		{"type not a string", `{"type":7}`, contract.RawUnknownType},
		{"unknown item type", `{"type":"item.completed","item":{"id":"item_1","type":"example_future_item"}}`, contract.RawUnknownType},
		{"thread without ID", `{"type":"thread.started"}`, contract.RawUnsupported},
		{"error without message", `{"type":"error"}`, contract.RawUnsupported},
		{"usage of the wrong shape", `{"type":"turn.completed","usage":{"input_tokens":"many"}}`, contract.RawUnsupported},
		{"item line without item", `{"type":"item.started"}`, contract.RawUnsupported},
		{"item without ID", `{"type":"item.completed","item":{"type":"agent_message","text":"a"}}`, contract.RawUnsupported},
		{"item with an empty ID", `{"type":"item.completed","item":{"id":"","type":"agent_message","text":"a"}}`, contract.RawUnsupported},
		{"item without type", `{"type":"item.completed","item":{"id":"item_1"}}`, contract.RawUnsupported},
		{"message without text", `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":null}}`, contract.RawUnsupported},
		{"error item without message", `{"type":"item.completed","item":{"id":"item_1","type":"error"}}`, contract.RawUnsupported},
		{"command without command", `{"type":"item.started","item":{"id":"item_1","type":"command_execution","status":"in_progress"}}`, contract.RawUnsupported},
		{"change without path", `{"type":"item.completed","item":{"id":"item_1","type":"file_change","changes":[{"kind":"add"}],"status":"completed"}}`, contract.RawUnsupported},
		{"MCP call without server", `{"type":"item.started","item":{"id":"item_1","type":"mcp_tool_call","tool":"t"}}`, contract.RawUnsupported},
		{"todo list without items", `{"type":"item.started","item":{"id":"item_1","type":"todo_list"}}`, contract.RawUnsupported},
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

func TestSourceTypeIsTheLineType(t *testing.T) {
	t.Parallel()
	events := normalize(t, `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"a"}}`)
	if ev := only(t, events, contract.KindMessage); ev.Source.Type != typeItemCompleted || ev.Source.RunnerTime != nil {
		t.Errorf("source = %+v, want type %s and no runner time", ev.Source, typeItemCompleted)
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
