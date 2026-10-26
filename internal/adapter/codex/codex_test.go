package codex

import (
	"encoding/json"
	"flag"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
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
	return adaptertest.Normalize(t, testAttempt, New(), lines...)
}

func only(t *testing.T, events []contract.Event, kind contract.EventKind) contract.Event {
	t.Helper()
	return adaptertest.Only(t, events, kind)
}

func ofKind(events []contract.Event, kind contract.EventKind) []contract.Event {
	return adaptertest.OfKind(events, kind)
}

func dump(events []contract.Event) string { return adaptertest.Dump(events) }

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

func TestFixturesMatchGolden(t *testing.T) {
	t.Parallel()
	adaptertest.Golden{
		Attempt:   testAttempt,
		Runner:    runnerName,
		New:       func() adapter.Runner { return New() },
		NewRepeat: newItemRepeat,
		Update:    *update,
	}.Match(t)
}

// TestFixturesRenderEachChangeOnce checks, over every fixture, that each
// result follows its call, that no call has two results, and that no
// message or tool call is rendered twice with the same content.
func TestFixturesRenderEachChangeOnce(t *testing.T) {
	t.Parallel()
	for _, f := range adaptertest.RunnerFixtures(t, runnerName) {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			calls := map[string]bool{}
			results := map[string]bool{}
			rendered := map[string]bool{}
			for _, ev := range normalize(t, adaptertest.FixtureLines(t, f.File)...) {
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

// newItemRepeat reports a later line of an item an earlier line showed:
// Codex prints an item again on each update, and an unchanged one shows
// nothing.
func newItemRepeat() adaptertest.Repeat {
	seen := map[string]bool{}
	return func(line string) bool {
		id, ok := itemID(line)
		if !ok {
			return false
		}
		repeated := seen[id]
		seen[id] = true
		return repeated
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
	adaptertest.FuzzSeeds(f, runnerName)
	f.Fuzz(func(t *testing.T, line string) {
		events := normalize(t, line, line)
		if line != "" && len(events) == 0 {
			t.Errorf("line %q produced no event", line)
		}
	})
}
