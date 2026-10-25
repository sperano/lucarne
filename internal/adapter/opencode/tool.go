package opencode

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Tool states (session.ts ToolState). run.ts prints a tool part only once
// it is completed or error; the others are handled for a line that has
// one anyway.
const (
	statusPending   = "pending"
	statusRunning   = "running"
	statusCompleted = "completed"
	statusError     = "error"
)

// toolState is a tool part's state: its input always, then output, title
// and metadata once completed, or the error text once failed. Metadata is
// each tool's own.
type toolState struct {
	Status      string          `json:"status"`
	Input       json.RawMessage `json:"input"`
	Output      *string         `json:"output"`
	Error       *string         `json:"error"`
	Title       string          `json:"title"`
	Metadata    json.RawMessage `json:"metadata"`
	Attachments json.RawMessage `json:"attachments"`
	Time        json.RawMessage `json:"time"`
}

// toolTime is the part's time; compacted is set only by compaction's
// prune, on a part already printed as completed.
type toolTime struct {
	Compacted json.RawMessage `json:"compacted"`
}

// pruned reports whether compaction pruned the part, which run.ts then
// prints a second time.
func (s *toolState) pruned() bool {
	var t toolTime
	if !isObject(s.Time) || json.Unmarshal(s.Time, &t) != nil {
		return false
	}
	return len(t.Compacted) > 0 && string(t.Compacted) != "null"
}

// exitMetadata is the bash tool's exit code, null when the command was
// aborted or timed out.
type exitMetadata struct {
	Exit *int `json:"exit"`
}

// renderTool adds a tool part: its call, then its result, attachments and
// file changes. A completed or failed part holds both the call and its
// outcome, so they share the line. A pruned part restates a call its first
// print gave, so its re-print is kept raw rather than given as a second
// call with the same callId.
func renderTool(p part, out *adapter.Emitter) bool {
	if p.Type != partTool || p.CallID == "" || p.Tool == "" || p.State == nil || p.State.Status == "" {
		return false
	}
	state := p.State
	if state.pruned() {
		out.RawLine(contract.RawUnsupported)
		return true
	}
	var cut adapter.Truncation
	call := &contract.ToolCall{
		CallID: cut.Text("callId", p.CallID, contract.MaxLabelBytes),
		Name:   cut.Text("name", p.Tool, contract.MaxLabelBytes),
		Title:  cut.Text("title", state.Title, contract.MaxLabelBytes),
	}
	adapter.SetToolInput(call, state.Input, &cut)
	out.Emit(contract.Event{Kind: contract.KindToolCall, ToolCall: call, Truncated: cut.Fields()})
	if state.Status == statusPending || state.Status == statusRunning {
		return true
	}
	out.Emit(toolResult(call.CallID, state))
	if state.Status != statusCompleted {
		return true
	}
	if attachments, ok := adapter.CompactJSON(state.Attachments); ok {
		out.Raw(contract.RawUnsupported, []byte(attachments))
	}
	for _, ev := range edits(p.Tool, call.CallID, state) {
		out.Emit(ev)
	}
	return true
}

// toolResult is the outcome of a completed or failed call: the output, or
// the error text OpenCode recorded (the thrown error's message, or "Tool
// execution aborted"). A state OpenCode does not define has an unknown
// outcome.
func toolResult(callID string, state *toolState) contract.Event {
	result := &contract.ToolResult{CallID: callID, Status: contract.ResultUnknown}
	var output *string
	switch state.Status {
	case statusCompleted:
		result.Status = contract.ResultOK
		output = state.Output
	case statusError:
		result.Status = contract.ResultError
		output = state.Error
	}
	var cut adapter.Truncation
	result.Output = cut.Optional("output", output, contract.MaxToolOutputBytes)
	var meta exitMetadata
	if isObject(state.Metadata) && json.Unmarshal(state.Metadata, &meta) == nil {
		result.ExitCode = meta.Exit
	}
	return contract.Event{Kind: contract.KindToolResult, ToolResult: result, Truncated: cut.Fields()}
}

func isObject(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '{'
}
