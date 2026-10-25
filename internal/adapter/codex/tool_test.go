package codex

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

// commandLine is a command_execution item line.
func commandLine(lineType, output, exit, status string) string {
	return fmt.Sprintf(`{"type":%q,"item":{"id":"item_1","type":"command_execution","command":"/bin/bash -lc 'go test'","aggregated_output":%q,"exit_code":%s,"status":%q}}`,
		lineType, output, exit, status)
}

func TestCommandLifecycle(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		commandLine(typeItemStarted, "", "null", "in_progress"),
		commandLine(typeItemCompleted, "FAIL\n", "1", "failed"),
	)
	call := only(t, events, contract.KindToolCall)
	if call.Source.Line != 1 || call.ToolCall.CallID != "item_1" || call.ToolCall.Name != itemCommand ||
		string(call.ToolCall.Input) != `{"command":"/bin/bash -lc 'go test'"}` {
		t.Errorf("call = %+v", call)
	}
	result := only(t, events, contract.KindToolResult)
	r := result.ToolResult
	if result.Source.Line != 2 || r.CallID != "item_1" || r.Status != contract.ResultError || *r.Output != "FAIL\n" || *r.ExitCode != 1 {
		t.Errorf("result = %+v", r)
	}
}

func TestCompletionWithoutStartGivesCallAndResult(t *testing.T) {
	t.Parallel()
	events := normalize(t, commandLine(typeItemCompleted, "", "null", "declined"))
	if len(events) != 2 || events[0].Kind != contract.KindToolCall || events[1].Kind != contract.KindToolResult {
		t.Fatalf("events = %s, want a call and its result", dump(events))
	}
	if r := events[1].ToolResult; r.Status != contract.ResultDeclined || r.Output != nil || r.ExitCode != nil {
		t.Errorf("declined result = %+v, want no output and no exit code", r)
	}
}

func TestCommandStatusesAndOutput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		output, exit, status string
		want                 contract.ToolResultStatus
		wantOutput           *string
	}{
		{"ok\n", "0", "completed", contract.ResultOK, ptrTo("ok\n")},
		{"", "0", "completed", contract.ResultOK, ptrTo("")},
		{"", "null", "in_progress", contract.ResultUnknown, nil},
		{"partial", "null", "in_progress", contract.ResultUnknown, ptrTo("partial")},
		{"", "null", "example_future_status", contract.ResultUnknown, nil},
	}
	for _, c := range cases {
		r := only(t, normalize(t, commandLine(typeItemCompleted, c.output, c.exit, c.status)), contract.KindToolResult).ToolResult
		if r.Status != c.want || !equalString(r.Output, c.wantOutput) {
			t.Errorf("%s with %q: result = %+v, want %s with output %v", c.status, c.output, r, c.want, c.wantOutput)
		}
	}
}

func TestStartedCallWithoutCompletionHasNoResult(t *testing.T) {
	t.Parallel()
	events := normalize(t, commandLine(typeItemStarted, "", "null", "in_progress"))
	if len(ofKind(events, contract.KindToolResult)) != 0 || len(ofKind(events, contract.KindToolCall)) != 1 {
		t.Errorf("events = %s, want the call alone", dump(events))
	}
}

func TestRepeatedCompletionIsRenderedOnce(t *testing.T) {
	t.Parallel()
	done := commandLine(typeItemCompleted, "ok\n", "0", "completed")
	events := normalize(t, commandLine(typeItemStarted, "", "null", "in_progress"), done, done)
	if len(events) != 2 || len(onLine(events, 3)) != 0 {
		t.Errorf("events = %s, want one call and one result", dump(events))
	}
	changed := normalize(t, done, commandLine(typeItemCompleted, "ok\nmore\n", "0", "completed"))
	if results := ofKind(changed, contract.KindToolResult); len(results) != 2 || len(ofKind(changed, contract.KindToolCall)) != 1 {
		t.Errorf("a changed completion gave %s, want a second result only", dump(changed))
	}
}

func todoLine(lineType string, done ...bool) string {
	var items []string
	for i, d := range done {
		items = append(items, fmt.Sprintf(`{"text":"step %d","completed":%t}`, i, d))
	}
	return fmt.Sprintf(`{"type":%q,"item":{"id":"item_1","type":"todo_list","items":[%s]}}`, lineType, strings.Join(items, ","))
}

func TestTodoListUpdates(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		todoLine(typeItemStarted, false, false),
		todoLine(typeItemUpdated, true, false),
		todoLine(typeItemUpdated, true, false),
		todoLine(typeItemCompleted, true, false),
	)
	calls := ofKind(events, contract.KindToolCall)
	if len(calls) != 2 || calls[0].Source.Line != 1 || calls[1].Source.Line != 2 {
		t.Fatalf("calls = %s, want the start and the one change", dump(calls))
	}
	if string(calls[1].ToolCall.Input) != `{"items":[{"text":"step 0","completed":true},{"text":"step 1","completed":false}]}` {
		t.Errorf("updated input = %s", calls[1].ToolCall.Input)
	}
	if len(onLine(events, 3)) != 0 {
		t.Errorf("the repeated update gave %s", dump(onLine(events, 3)))
	}
	result := only(t, events, contract.KindToolResult)
	if result.Source.Line != 4 || result.ToolResult.Status != contract.ResultUnknown || result.ToolResult.Output != nil {
		t.Errorf("todo result = %+v", result)
	}
}

func TestFileChangeEdits(t *testing.T) {
	t.Parallel()
	line := func(status string) string {
		return `{"type":"item.completed","item":{"id":"item_3","type":"file_change","changes":[{"path":"/w/a.go","kind":"add"},{"path":"/w/b.go","kind":"update"},{"path":"/w/c.go","kind":"delete"},{"path":"/w/d.go","kind":"example"}],"status":"` + status + `"}}`
	}
	events := normalize(t, line("completed"))
	edits := ofKind(events, contract.KindEdit)
	want := []contract.EditChange{contract.ChangeAdd, contract.ChangeUpdate, contract.ChangeDelete, contract.ChangeUnknown}
	if len(edits) != len(want) {
		t.Fatalf("edits = %s", dump(edits))
	}
	for i, ev := range edits {
		e := ev.Edit
		if e.CallID != "item_3" || e.Change != want[i] || e.Diff != nil || e.Before != nil || e.After != nil {
			t.Errorf("edit %d = %+v, want %s with no content", i, e, want[i])
		}
	}
	if r := only(t, events, contract.KindToolResult).ToolResult; r.Status != contract.ResultOK || r.Output != nil {
		t.Errorf("result = %+v", r)
	}
	failed := normalize(t, line("failed"))
	if len(ofKind(failed, contract.KindEdit)) != 0 || only(t, failed, contract.KindToolResult).ToolResult.Status != contract.ResultError {
		t.Errorf("a failed change gave %s, want an error result and no edit", dump(failed))
	}
}

func mcpLine(result, failure, status string) string {
	return `{"type":"item.completed","item":{"id":"item_2","type":"mcp_tool_call","server":"tracker","tool":"get_item","arguments":{"id":7},"result":` +
		result + `,"error":` + failure + `,"status":"` + status + `"}}`
}

func TestMCPToolCalls(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, result, failure, status string
		want                          contract.ToolResultStatus
		output                        *string
	}{
		{"text", `{"content":[{"type":"text","text":"a"},{"type":"text","text":"b"}],"structured_content":null}`, "null", "completed", contract.ResultOK, ptrTo("a\nb")},
		{"not only text", `{"content":[{"type":"text","text":"a"},{"type":"image","data":"x"}]}`, "null", "completed", contract.ResultOK, ptrTo(`{"content":[{"type":"text","text":"a"},{"type":"image","data":"x"}]}`)},
		{"structured only", `{"content":[],"structured_content":{"n":1}}`, "null", "completed", contract.ResultOK, ptrTo(`{"content":[],"structured_content":{"n":1}}`)},
		{"error", "null", `{"message":"not found"}`, "failed", contract.ResultError, ptrTo("not found")},
		{"nothing", "null", "null", "in_progress", contract.ResultUnknown, nil},
	}
	for _, c := range cases {
		events := normalize(t, mcpLine(c.result, c.failure, c.status))
		if in := string(only(t, events, contract.KindToolCall).ToolCall.Input); in != `{"arguments":{"id":7},"server":"tracker","tool":"get_item"}` {
			t.Errorf("%s: input = %s", c.name, in)
		}
		r := only(t, events, contract.KindToolResult).ToolResult
		if r.Status != c.want || !equalString(r.Output, c.output) {
			t.Errorf("%s: result = %+v, want %s with output %v", c.name, r, c.want, c.output)
		}
	}
}

func TestCollabToolCall(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"item.started","item":{"id":"item_5","type":"collab_tool_call","tool":"spawn_agent","sender_thread_id":"t","receiver_thread_ids":[],"prompt":null,"agents_states":{},"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_5","type":"collab_tool_call","tool":"spawn_agent","sender_thread_id":"t","receiver_thread_ids":["u"],"prompt":null,"agents_states":{"u":{"status":"running","message":null}},"status":"completed"}}`,
	)
	if calls := ofKind(events, contract.KindToolCall); len(calls) != 2 {
		t.Errorf("calls = %s, want the start and the changed receivers", dump(calls))
	}
	r := only(t, events, contract.KindToolResult).ToolResult
	if r.Status != contract.ResultOK || *r.Output != `{"u":{"status":"running","message":null}}` {
		t.Errorf("result = %+v", r)
	}
}

func TestToolInputAndOutputAreBounded(t *testing.T) {
	t.Parallel()
	command := strings.Repeat("x", contract.MaxToolInputBytes)
	output := strings.Repeat("é", contract.MaxToolOutputBytes)
	line := fmt.Sprintf(`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":%q,"aggregated_output":%q,"exit_code":0,"status":"completed"}}`, command, output)
	events := normalize(t, line)
	call := only(t, events, contract.KindToolCall)
	if call.ToolCall.Input != nil || len(call.ToolCall.InputText) != contract.MaxToolInputBytes || call.Truncated[0] != "inputText" {
		t.Errorf("call input %d bytes, inputText %d bytes, truncated %v", len(call.ToolCall.Input), len(call.ToolCall.InputText), call.Truncated)
	}
	result := only(t, events, contract.KindToolResult)
	if len(*result.ToolResult.Output) > contract.MaxToolOutputBytes || result.Truncated[0] != "output" {
		t.Errorf("output %d bytes, truncated %v", len(*result.ToolResult.Output), result.Truncated)
	}
}

func ptrTo(s string) *string { return &s }

func equalString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
