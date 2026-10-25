package opencode

import (
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
)

func toolLine(tool, state string) string {
	return `{"type":"tool_use","part":{"type":"tool","callID":"call_a","tool":"` + tool + `","state":` + state + `}}`
}

func TestToolCallAndResultShareTheLine(t *testing.T) {
	t.Parallel()
	events := normalize(t, toolLine("bash",
		`{"status":"completed","input":{"command":"false"},"output":"","title":"false","metadata":{"exit":1}}`))
	if len(events) != 2 {
		t.Fatalf("events = %s", adaptertest.Dump(events))
	}
	call, result := events[0], events[1]
	if call.ToolCall == nil || call.ToolCall.CallID != "call_a" || call.ToolCall.Name != "bash" ||
		call.ToolCall.Title != "false" || string(call.ToolCall.Input) != `{"command":"false"}` {
		t.Errorf("call = %+v", call.ToolCall)
	}
	r := result.ToolResult
	if r == nil || r.CallID != "call_a" || r.Status != contract.ResultOK || r.Output == nil || *r.Output != "" ||
		r.ExitCode == nil || *r.ExitCode != 1 {
		t.Errorf("a completed call exiting 1 = %+v, want ok with exit code 1", r)
	}
	if call.Source.Part != 0 || result.Source.Part != 1 {
		t.Errorf("parts = %d, %d", call.Source.Part, result.Source.Part)
	}
}

func TestToolStatuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		state  string
		want   contract.ToolResultStatus
		output string
		events int
	}{
		{"error", `{"status":"error","input":{},"error":"Tool execution aborted","metadata":{"exit":null,"interrupted":true}}`,
			contract.ResultError, "Tool execution aborted", 2},
		{"unknown status", `{"status":"example-future","input":{}}`, contract.ResultUnknown, "", 2},
		{"running", `{"status":"running","input":{"command":"sleep 9"}}`, "", "", 1},
		{"pending", `{"status":"pending","input":{},"raw":""}`, "", "", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			events := normalize(t, toolLine("bash", c.state))
			if len(events) != c.events || events[0].Kind != contract.KindToolCall {
				t.Fatalf("events = %s", adaptertest.Dump(events))
			}
			if c.events == 1 {
				return
			}
			r := events[1].ToolResult
			if r.Status != c.want || r.ExitCode != nil {
				t.Errorf("result = %+v, want status %s and no exit code", r, c.want)
			}
			if got := r.Output; (c.output == "") != (got == nil) || (got != nil && *got != c.output) {
				t.Errorf("output = %v, want %q", got, c.output)
			}
		})
	}
}

func TestToolMetadataOfAnotherShapeGivesNoExitCode(t *testing.T) {
	t.Parallel()
	r := only(t, normalize(t, toolLine("bash", `{"status":"completed","input":{},"output":"x","metadata":{"exit":"1"}}`)),
		contract.KindToolResult).ToolResult
	if r.Status != contract.ResultOK || r.ExitCode != nil {
		t.Errorf("result = %+v", r)
	}
}

func TestToolInputAndOutputAreBounded(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", contract.MaxToolOutputBytes+1)
	events := normalize(t, toolLine("write", `{"status":"completed","input":{"content":"`+big+`"},"output":"`+big+`"}`))
	call, result := events[0], events[1]
	if call.ToolCall.Input != nil || len(call.ToolCall.InputText) != contract.MaxToolInputBytes || call.Truncated[0] != "inputText" {
		t.Errorf("call input of %d bytes, input text %d, truncated %v", len(call.ToolCall.Input), len(call.ToolCall.InputText), call.Truncated)
	}
	if len(*result.ToolResult.Output) != contract.MaxToolOutputBytes || result.Truncated[0] != "output" {
		t.Errorf("output of %d bytes, truncated %v", len(*result.ToolResult.Output), result.Truncated)
	}
}

func TestToolAttachmentsAreRaw(t *testing.T) {
	t.Parallel()
	events := normalize(t, toolLine("read",
		`{"status":"completed","input":{},"output":"Image read.","attachments":[{"type":"file","mime":"image/png","url":"data:image/png;base64,AA=="}]}`))
	if len(events) != 3 || events[2].Kind != contract.KindRaw || events[2].Raw.Reason != contract.RawUnsupported ||
		!strings.Contains(events[2].Raw.Text, "image/png") {
		t.Errorf("events = %s", adaptertest.Dump(events))
	}
}
