package claude

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Content block types.
const (
	blockText             = "text"
	blockThinking         = "thinking"
	blockRedactedThinking = "redacted_thinking"
	blockToolUse          = "tool_use"
	blockToolResult       = "tool_result"
	blockImage            = "image"
	blockDocument         = "document"
	// blockToolReference names a tool a tool search loaded, inside a
	// tool_result's content.
	blockToolReference = "tool_reference"
)

// unsupportedBlocks are block types the parser knows and does not render:
// encrypted reasoning, and binary content.
var unsupportedBlocks = map[string]bool{blockRedactedThinking: true, blockImage: true, blockDocument: true}

// messageLine is an assistant or user line.
type messageLine struct {
	Message *apiMessage `json:"message"`
	// ParentToolUseID is the Task call a subagent's line belongs to.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// Error marks an API error the CLI turned into an assistant message,
	// such as "rate_limit".
	Error string `json:"error"`
	// ToolUseResult is the structured output of the tool whose result the
	// user line carries.
	ToolUseResult json.RawMessage `json:"tool_use_result"`
}

// apiMessage is an API message. StopReason is empty while the message is
// partial: stream-json prints one line per content block as each ends, and
// only the message's last line can carry the reason it stopped.
type apiMessage struct {
	ID         string          `json:"id"`
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stop_reason"`
	Usage      *apiUsage       `json:"usage"`
}

// block is any content block; each type sets its own fields.
type block struct {
	Type      string          `json:"type"`
	Text      *string         `json:"text"`
	Thinking  *string         `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   *bool           `json:"is_error"`
	ToolName  string          `json:"tool_name"`
}

// decodeMessageLine decodes an assistant or user line with a message.
func decodeMessageLine(text []byte) (messageLine, bool) {
	var line messageLine
	if json.Unmarshal(text, &line) != nil || line.Message == nil {
		return messageLine{}, false
	}
	return line, true
}

// contentBlocks splits a content array into its blocks, undecoded.
func contentBlocks(content json.RawMessage) ([]json.RawMessage, bool) {
	var blocks []json.RawMessage
	if json.Unmarshal(content, &blocks) != nil || blocks == nil {
		return nil, false
	}
	return blocks, true
}

// renderAssistant adds one event per content block, then the message's
// usage once it is final.
func (p *Parser) renderAssistant(text []byte, out *adapter.Emitter) bool {
	line, ok := decodeMessageLine(text)
	if !ok {
		return false
	}
	blocks, ok := contentBlocks(line.Message.Content)
	if !ok {
		return false
	}
	reported := false
	for _, raw := range blocks {
		reported = p.assistantBlock(raw, line, out) || reported
	}
	if line.Error != "" && !reported {
		out.Emit(apiError(line.Error, line.Error))
	}
	if line.Message.StopReason != "" {
		if usage, ok := line.Message.Usage.usage(contract.ScopeMessage); ok {
			out.Emit(contract.Event{Kind: contract.KindUsage, Usage: &usage})
		}
	}
	return true
}

// assistantBlock adds the event of one block and reports whether it was
// the text of an API error.
func (p *Parser) assistantBlock(raw json.RawMessage, line messageLine, out *adapter.Emitter) bool {
	var b block
	if json.Unmarshal(raw, &b) != nil {
		out.Raw(contract.RawUnsupported, raw)
		return false
	}
	switch {
	case b.Type == blockText && b.Text != nil && line.Error != "":
		out.Emit(apiError(*b.Text, line.Error))
		return true
	case b.Type == blockText && b.Text != nil:
		out.Emit(message(contract.RoleAssistant, contract.MessageText, *b.Text, line.Message.ID))
	case b.Type == blockThinking && b.Thinking != nil:
		out.Emit(message(contract.RoleAssistant, contract.MessageReasoning, *b.Thinking, line.Message.ID))
	case b.Type == blockToolUse && b.ID != "" && b.Name != "":
		p.calls.open(b.ID, b.Name)
		out.Emit(toolCall(b, line.ParentToolUseID))
	default:
		out.Raw(blockRawReason(b.Type), raw)
	}
	return false
}

// blockRawReason says why a block was not rendered.
func blockRawReason(blockType string) contract.RawReason {
	switch blockType {
	case blockText, blockThinking, blockToolUse, blockToolResult:
		return contract.RawUnsupported
	}
	if unsupportedBlocks[blockType] {
		return contract.RawUnsupported
	}
	return contract.RawUnknownType
}

func message(role contract.MessageRole, kind contract.MessageType, text, id string) contract.Event {
	var cut adapter.Truncation
	m := &contract.Message{
		Role:      role,
		Type:      kind,
		Text:      cut.Text("text", text, contract.MaxTextBytes),
		MessageID: cut.Text("messageId", id, contract.MaxLabelBytes),
	}
	return contract.Event{Kind: contract.KindMessage, Message: m, Truncated: cut.Fields()}
}

// apiError is an API error the CLI printed as an assistant message; code is
// the line's error field.
func apiError(text, code string) contract.Event {
	var cut adapter.Truncation
	info := &contract.ErrorInfo{
		Message: cut.Text("message", text, contract.MaxTextBytes),
		Code:    cut.Text("code", code, contract.MaxLabelBytes),
	}
	return contract.Event{Kind: contract.KindError, Error: info, Truncated: cut.Fields()}
}

// toolCall keeps the input as JSON when it fits, else its cut serialization.
func toolCall(b block, parent string) contract.Event {
	var cut adapter.Truncation
	call := &contract.ToolCall{
		CallID:       cut.Text("callId", b.ID, contract.MaxLabelBytes),
		Name:         cut.Text("name", b.Name, contract.MaxLabelBytes),
		ParentCallID: cut.Text("parentCallId", parent, contract.MaxLabelBytes),
	}
	adapter.SetToolInput(call, b.Input, &cut)
	return contract.Event{Kind: contract.KindToolCall, ToolCall: call, Truncated: cut.Fields()}
}
