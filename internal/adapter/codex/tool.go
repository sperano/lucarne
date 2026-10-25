package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Tool item types (exec_events.rs ThreadItemDetails). The item type is the
// tool call's name.
const (
	itemCommand    = "command_execution"
	itemFileChange = "file_change"
	itemMCPTool    = "mcp_tool_call"
	itemCollabTool = "collab_tool_call"
	itemWebSearch  = "web_search"
	itemTodoList   = "todo_list"
)

// Item statuses (CommandExecutionStatus, PatchApplyStatus,
// McpToolCallStatus, CollabToolCallStatus). in_progress, on a completed
// item, is one a turn ended before it finished.
const (
	statusCompleted = "completed"
	statusFailed    = "failed"
	statusDeclined  = "declined"
)

// File change kinds (PatchChangeKind).
const (
	kindAdd    = "add"
	kindUpdate = "update"
	kindDelete = "delete"
)

// mcpTextSeparator joins the text blocks of an MCP tool's result.
const mcpTextSeparator = "\n"

// toolItem says how an item type becomes a tool call and its result.
type toolItem struct {
	// outcome names the fields that report the call's outcome. The item's
	// other fields, but id, type and status, are the call's input.
	outcome []string
	// valid reports whether the item has the fields its type requires;
	// nil accepts any.
	valid func(item) bool
	// output is the result's output, nil when the item carries none; nil
	// for a type whose items carry no output.
	output func(item) *string
}

var toolItems = map[string]toolItem{
	itemCommand:    {outcome: []string{"aggregated_output", "exit_code"}, valid: validCommand, output: commandOutput},
	itemFileChange: {valid: validFileChange},
	itemMCPTool:    {outcome: []string{"result", "error"}, valid: validMCPTool, output: mcpOutput},
	itemCollabTool: {outcome: []string{"agents_states"}, output: jsonField("agents_states")},
	itemWebSearch:  {outcome: []string{"results"}, output: jsonField("results")},
	itemTodoList:   {valid: validTodoList},
}

// renderTool renders a tool item: its call when the call's input is new
// or changed, and, on item.completed, its result (and a successful file
// change's edits) unless the same result was shown already.
func (p *Parser) renderTool(completed bool, it item, tool toolItem, out *adapter.Emitter) contract.RawReason {
	if tool.valid != nil && !tool.valid(it) {
		return contract.RawUnsupported
	}
	input := callInput(it, tool.outcome)
	seen := p.items.get(it.id)
	if !seen.sameContent(digestOf(it.kind, input)) {
		out.Emit(toolCall(it, input))
	}
	if !completed {
		return ""
	}
	status := resultStatus(it)
	if seen.sameOutcome(outcomeDigest(it, status, tool.outcome)) {
		return ""
	}
	out.Emit(toolResult(it, status, tool.output))
	if it.kind == itemFileChange && status == contract.ResultOK {
		for _, ev := range edits(it) {
			out.Emit(ev)
		}
	}
	return ""
}

// callInput is the call's input: the item's fields that are neither its
// envelope nor its outcome, as a JSON object. HTML characters are kept as
// Codex printed them: json.Marshal would escape the &, < and > of shell
// commands, which costs input budget and shows in a cut inputText.
func callInput(it item, outcome []string) string {
	input := maps.Clone(it.fields)
	for _, name := range append([]string{fieldID, fieldType, fieldStatus}, outcome...) {
		delete(input, name)
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(input); err != nil {
		// Unreachable: every value is raw JSON the item's decoding
		// validated. Fail loudly rather than render a made-up input.
		panic(fmt.Sprintf("codex: encoding tool input: %v", err))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func toolCall(it item, input string) contract.Event {
	var cut adapter.Truncation
	call := &contract.ToolCall{
		CallID: cut.Text("callId", it.id, contract.MaxLabelBytes),
		Name:   cut.Text("name", it.kind, contract.MaxLabelBytes),
	}
	cut.ToolInput(call, input)
	return contract.Event{Kind: contract.KindToolCall, ToolCall: call, Truncated: cut.Fields()}
}

func toolResult(it item, status contract.ToolResultStatus, output func(item) *string) contract.Event {
	var cut adapter.Truncation
	result := &contract.ToolResult{
		CallID: cut.Text("callId", it.id, contract.MaxLabelBytes),
		Status: status,
	}
	if output != nil {
		result.Output = cut.Optional("output", output(it), contract.MaxToolOutputBytes)
	}
	var exit int
	if it.kind == itemCommand && it.field("exit_code", &exit) {
		result.ExitCode = &exit
	}
	return contract.Event{Kind: contract.KindToolResult, ToolResult: result, Truncated: cut.Fields()}
}

// resultStatus maps the item's status; an item without one (web_search,
// todo_list) or still in progress has an unknown outcome.
func resultStatus(it item) contract.ToolResultStatus {
	var status string
	it.field(fieldStatus, &status)
	switch status {
	case statusCompleted:
		return contract.ResultOK
	case statusFailed:
		return contract.ResultError
	case statusDeclined:
		return contract.ResultDeclined
	}
	return contract.ResultUnknown
}

func outcomeDigest(it item, status contract.ToolResultStatus, outcome []string) digest {
	parts := []string{string(status)}
	for _, name := range outcome {
		parts = append(parts, string(it.fields[name]))
	}
	return digestOf(parts...)
}

// commandOutput is the command's stdout and stderr as Codex aggregated
// them. Codex prints "" for an output it does not have, so an empty one
// without an exit code (a declined or unfinished command) is none.
func commandOutput(it item) *string {
	var output string
	var exit int
	if !it.field("aggregated_output", &output) || (output == "" && !it.field("exit_code", &exit)) {
		return nil
	}
	return &output
}

// mcpResult is an MCP tool call's result.
type mcpResult struct {
	Content []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	} `json:"content"`
}

// mcpOutput is the error's message, or the result's text when all its
// content is text, or else the result as JSON.
func mcpOutput(it item) *string {
	var failure errorPayload
	if it.field("error", &failure) && failure.Message != nil {
		return failure.Message
	}
	var result mcpResult
	if it.field("result", &result) && len(result.Content) > 0 {
		texts := make([]string, 0, len(result.Content))
		for _, block := range result.Content {
			if block.Type != "text" || block.Text == nil {
				return jsonField("result")(it)
			}
			texts = append(texts, *block.Text)
		}
		joined := strings.Join(texts, mcpTextSeparator)
		return &joined
	}
	return jsonField("result")(it)
}

// jsonField returns an output function giving the named field as compact
// JSON, nil when the item does not have it or it is null.
func jsonField(name string) func(item) *string {
	return func(it item) *string {
		raw := it.fields[name]
		if isNull(raw) {
			return nil
		}
		var buf bytes.Buffer
		if json.Compact(&buf, raw) != nil {
			return nil
		}
		s := buf.String()
		return &s
	}
}

// fileChange is one entry of a file_change item's changes.
type fileChange struct {
	Path *string `json:"path"`
	Kind string  `json:"kind"`
}

func fileChanges(it item) ([]fileChange, bool) {
	var changes []fileChange
	if !it.field("changes", &changes) {
		return nil, false
	}
	for _, c := range changes {
		if c.Path == nil || *c.Path == "" {
			return nil, false
		}
	}
	return changes, true
}

// edits is one edit per change, with the path and kind only: Codex prints
// no diff and no content.
func edits(it item) []contract.Event {
	changes, _ := fileChanges(it)
	events := make([]contract.Event, 0, len(changes))
	for _, c := range changes {
		var cut adapter.Truncation
		edit := &contract.Edit{
			CallID: cut.Text("callId", it.id, contract.MaxLabelBytes),
			Path:   cut.Text("path", *c.Path, contract.MaxPathBytes),
			Change: editChange(c.Kind),
		}
		events = append(events, contract.Event{Kind: contract.KindEdit, Edit: edit, Truncated: cut.Fields()})
	}
	return events
}

func editChange(kind string) contract.EditChange {
	switch kind {
	case kindAdd:
		return contract.ChangeAdd
	case kindUpdate:
		return contract.ChangeUpdate
	case kindDelete:
		return contract.ChangeDelete
	}
	return contract.ChangeUnknown
}

func validCommand(it item) bool {
	var command string
	return it.field("command", &command)
}

func validFileChange(it item) bool {
	_, ok := fileChanges(it)
	return ok
}

func validMCPTool(it item) bool {
	var server, tool string
	return it.field("server", &server) && it.field("tool", &tool)
}

func validTodoList(it item) bool {
	var items []json.RawMessage
	return it.field("items", &items)
}
