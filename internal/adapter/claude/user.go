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

// toolResult adds the result, its content blocks it does not render, and
// the edit a successful edit or write made.
func (p *Parser) toolResult(b block, structured json.RawMessage, out *adapter.Emitter) {
	tool := p.calls.close(b.ToolUseID)
	status := contract.ResultOK
	if b.IsError != nil && *b.IsError {
		status = contract.ResultError
	}
	var cut adapter.Truncation
	content := resultContent(b.Content, &cut)
	result := &contract.ToolResult{
		CallID: cut.Text("callId", b.ToolUseID, contract.MaxLabelBytes),
		Status: status,
		Output: cut.Optional("output", content.output, contract.MaxToolOutputBytes),
		Media:  content.media,
	}
	out.Emit(contract.Event{Kind: contract.KindToolResult, ToolResult: result, Truncated: cut.Fields()})
	for _, other := range content.others {
		out.Raw(contract.RawUnsupported, other)
	}
	if status != contract.ResultOK {
		return
	}
	if ev, ok := editEvent(tool, result.CallID, structured); ok {
		out.Emit(ev)
	}
}

// toolContent is what a tool result's content holds: its text, nil when
// it has none, its media, and the blocks it does not render, undecoded.
type toolContent struct {
	output *string
	media  []contract.Media
	others []json.RawMessage
}

// resultContent splits a tool result's content: the content string, or its
// text blocks and the names of its tool references, one per line, as the
// output; its image and document blocks, at most contract.MaxMedia of
// them, as media; the rest as others.
func resultContent(content json.RawMessage, cut *adapter.Truncation) toolContent {
	if len(content) == 0 || string(content) == "null" {
		return toolContent{}
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return toolContent{output: &s}
	}
	blocks, ok := contentBlocks(content)
	if !ok {
		return toolContent{others: []json.RawMessage{content}}
	}
	var c toolContent
	var texts []string
	for _, raw := range blocks {
		var b block
		if json.Unmarshal(raw, &b) != nil {
			c.others = append(c.others, raw)
		} else if text, ok := resultText(b); ok {
			texts = append(texts, text)
		} else if !c.addMedia(b, cut) {
			c.others = append(c.others, raw)
		}
	}
	if texts != nil || len(blocks) == 0 {
		joined := strings.Join(texts, textSeparator)
		c.output = &joined
	}
	return c
}

// addMedia adds b to the media when it is an image or document block it
// can read; one past contract.MaxMedia is dropped and media marked cut.
func (c *toolContent) addMedia(b block, cut *adapter.Truncation) bool {
	m, ok := resultMedia(b, cut)
	if !ok {
		return false
	}
	if len(c.media) == contract.MaxMedia {
		cut.Mark(mediaField)
		return true
	}
	c.media = append(c.media, m)
	return true
}

// resultText is the text a tool result's content block shows: a text
// block's text, or the name of the tool a tool reference loaded.
func resultText(b block) (string, bool) {
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
