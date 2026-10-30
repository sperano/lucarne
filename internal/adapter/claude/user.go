package claude

import (
	"encoding/json"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// textSeparator joins the text blocks of one tool result.
const textSeparator = "\n"

// renderUser adds the events of a user line: its text, or one event per
// content block, tool results with the edit their tool made.
func (p *Parser) renderUser(text []byte, out *adapter.Emitter) bool {
	line, ok := decodeMessageLine(text)
	if !ok {
		return false
	}
	var prompt string
	if json.Unmarshal(line.Message.Content, &prompt) == nil {
		out.Emit(message(contract.RoleUser, contract.MessageText, prompt, line.Message.ID))
		return true
	}
	blocks, ok := contentBlocks(line.Message.Content)
	if !ok {
		return false
	}
	// tool_use_result belongs to the line's only tool result; with several,
	// it cannot be told whose it is.
	structured := line.ToolUseResult
	if countToolResults(blocks) != 1 {
		structured = nil
	}
	for _, raw := range blocks {
		p.userBlock(raw, line.Message.ID, structured, out)
	}
	return true
}

func (p *Parser) userBlock(raw json.RawMessage, id string, structured json.RawMessage, out *adapter.Emitter) {
	var b block
	if json.Unmarshal(raw, &b) != nil {
		out.Raw(contract.RawUnsupported, raw)
		return
	}
	switch {
	case b.Type == blockText && b.Text != nil:
		out.Emit(message(contract.RoleUser, contract.MessageText, *b.Text, id))
	case b.Type == blockToolResult && b.ToolUseID != "":
		p.toolResult(b, structured, out)
	default:
		out.Raw(blockRawReason(b.Type), raw)
	}
}

// toolResult adds the result, its content blocks that are not text, and
// the edit a successful edit or write made.
func (p *Parser) toolResult(b block, structured json.RawMessage, out *adapter.Emitter) {
	tool := p.calls.close(b.ToolUseID)
	status := contract.ResultOK
	if b.IsError != nil && *b.IsError {
		status = contract.ResultError
	}
	output, others := resultOutput(b.Content)
	var cut adapter.Truncation
	result := &contract.ToolResult{
		CallID: cut.Text("callId", b.ToolUseID, contract.MaxLabelBytes),
		Status: status,
		Output: cut.Optional("output", output, contract.MaxToolOutputBytes),
	}
	out.Emit(contract.Event{Kind: contract.KindToolResult, ToolResult: result, Truncated: cut.Fields()})
	for _, other := range others {
		out.Raw(contract.RawUnsupported, other)
	}
	if status != contract.ResultOK {
		return
	}
	if ev, ok := editEvent(tool, result.CallID, structured); ok {
		out.Emit(ev)
	}
}

// resultOutput is a tool result's text: the content string, or its text
// blocks and the names of its tool references, one per line, nil when it has
// none of them. The other blocks (images) are returned undecoded.
func resultOutput(content json.RawMessage) (*string, []json.RawMessage) {
	if len(content) == 0 || string(content) == "null" {
		return nil, nil
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return &s, nil
	}
	blocks, ok := contentBlocks(content)
	if !ok {
		return nil, []json.RawMessage{content}
	}
	var texts []string
	var others []json.RawMessage
	for _, raw := range blocks {
		if text, ok := resultText(raw); ok {
			texts = append(texts, text)
		} else {
			others = append(others, raw)
		}
	}
	if texts == nil && len(blocks) > 0 {
		return nil, others
	}
	joined := strings.Join(texts, textSeparator)
	return &joined, others
}

// resultText is the text a tool result's content block shows: a text
// block's text, or the name of the tool a tool reference loaded.
func resultText(raw json.RawMessage) (string, bool) {
	var b block
	if json.Unmarshal(raw, &b) != nil {
		return "", false
	}
	switch {
	case b.Type == blockText && b.Text != nil:
		return *b.Text, true
	case b.Type == blockToolReference && b.ToolName != "":
		return b.ToolName, true
	}
	return "", false
}

func countToolResults(blocks []json.RawMessage) int {
	n := 0
	for _, raw := range blocks {
		var b block
		if json.Unmarshal(raw, &b) == nil && b.Type == blockToolResult {
			n++
		}
	}
	return n
}
