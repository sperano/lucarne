package claude

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestToolResultsCorrelateByID(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"toolu_a","name":"Read","input":{}}]}}`,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"toolu_b","name":"Bash","input":{}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_b","content":"b","is_error":true},{"type":"tool_result","tool_use_id":"toolu_a","content":"a"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_unseen","content":"c","is_error":false}]}}`,
	)
	results := ofKind(events, contract.KindToolResult)
	want := []struct {
		id, output string
		status     contract.ToolResultStatus
	}{
		{"toolu_b", "b", contract.ResultError},
		{"toolu_a", "a", contract.ResultOK},
		{"toolu_unseen", "c", contract.ResultOK},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d: %s", len(results), len(want), dump(events))
	}
	for i, r := range results {
		got := r.ToolResult
		if got.CallID != want[i].id || got.Status != want[i].status || got.Output == nil || *got.Output != want[i].output {
			t.Errorf("result %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestToolResultContent(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"one"},{"type":"image","source":{}},{"type":"text","text":"two"}]}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":""}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t4","content":[{"type":"image","source":{}}]}]}}`,
	)
	if len(events) != 6 {
		t.Fatalf("got %d events, want 6: %s", len(events), dump(events))
	}
	if out := events[0].ToolResult.Output; out == nil || *out != "one\ntwo" {
		t.Errorf("text blocks = %v, want them joined", out)
	}
	if events[1].Kind != contract.KindRaw || events[1].Raw.Text != `{"type":"image","source":{}}` {
		t.Errorf("the image block is %+v, want it raw after the result", events[1])
	}
	if out := events[2].ToolResult.Output; out == nil || *out != "" {
		t.Errorf("an empty output = %v, want empty, not absent", out)
	}
	if out := events[3].ToolResult.Output; out != nil {
		t.Errorf("a result without content has output %q, want none", *out)
	}
	if out := events[4].ToolResult.Output; out != nil || events[5].Kind != contract.KindRaw {
		t.Errorf("an image-only result = %v then %s, want no output then raw", out, events[5].Kind)
	}
}

func TestToolReferencesAreResultText(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"t1","name":"ToolSearch","input":{"query":"select:WebSearch,WebFetch"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"tool_reference","tool_name":"WebSearch"},{"type":"tool_reference","tool_name":"WebFetch"}]}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":[{"type":"text","text":"Loaded:"},{"type":"tool_reference","tool_name":"mcp__example__read"},{"type":"tool_reference"}]}]}}`,
	)
	if raw := ofKind(events, contract.KindRaw); len(raw) != 1 || raw[0].Raw.Text != `{"type":"tool_reference"}` {
		t.Fatalf("raw events = %s, want only the reference without a name", dump(raw))
	}
	results := ofKind(events, contract.KindToolResult)
	for i, want := range []string{"WebSearch\nWebFetch", "Loaded:\nmcp__example__read"} {
		if out := results[i].ToolResult.Output; out == nil || *out != want {
			t.Errorf("result %d output = %v, want %q", i, out, want)
		}
	}
}

func TestUserText(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"user","message":{"role":"user","content":"Fix the greeting."}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
	)
	for i, want := range []string{"Fix the greeting.", "[Request interrupted by user]"} {
		m := events[i].Message
		if m == nil || m.Role != contract.RoleUser || m.Text != want {
			t.Errorf("event %d = %s, want the user message %q", i, dump(events[i:i+1]), want)
		}
	}
}
