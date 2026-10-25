package codex

import (
	"bytes"
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Item types that are not tool calls (exec_events.rs ThreadItemDetails).
const (
	itemAgentMessage = "agent_message"
	itemReasoning    = "reasoning"
	itemError        = "error"
)

// Fields every item has.
const (
	fieldID     = "id"
	fieldType   = "type"
	fieldStatus = "status"
)

// item is a decoded thread item: its ID, its type and its fields, values
// undecoded.
type item struct {
	id     string
	kind   string
	fields map[string]json.RawMessage
}

// decodeItem decodes an item line's item. The ID is the first "id" of the
// object: a web_search item serializes its search's own id after it, and
// a map keeps the last one, so firstID reads the object a second time on
// purpose.
func decodeItem(raw json.RawMessage) (item, bool) {
	var it item
	if json.Unmarshal(raw, &it.fields) != nil || it.fields == nil {
		return item{}, false
	}
	id, ok := firstID(raw)
	if !ok || id == "" || !it.field(fieldType, &it.kind) || it.kind == "" {
		return item{}, false
	}
	it.id = id
	return it, true
}

// firstID is the value of the object's first "id" member, ok false when
// it has none or it is not a string.
func firstID(raw json.RawMessage) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return "", false
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return "", false
		}
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			return "", false
		}
		if key == fieldID {
			var id string
			if json.Unmarshal(value, &id) != nil {
				return "", false
			}
			return id, true
		}
	}
	return "", false
}

// field decodes the named field into v and reports whether the item has
// it, not null, and of v's type.
func (it item) field(name string, v any) bool {
	raw, ok := it.fields[name]
	return ok && !isNull(raw) && json.Unmarshal(raw, v) == nil
}

func isNull(raw json.RawMessage) bool { return len(raw) == 0 || string(raw) == "null" }

// renderItem renders an item line; completed says whether it is the
// item's item.completed line.
func (p *Parser) renderItem(completed bool, raw json.RawMessage, out *adapter.Emitter) contract.RawReason {
	it, ok := decodeItem(raw)
	if !ok {
		return contract.RawUnsupported
	}
	switch it.kind {
	case itemAgentMessage, itemReasoning:
		return p.renderMessage(it, out)
	case itemError:
		return p.renderErrorItem(it, out)
	}
	tool, ok := toolItems[it.kind]
	if !ok {
		return contract.RawUnknownType
	}
	return p.renderTool(completed, it, tool, out)
}

// renderMessage renders an agent message or a reasoning summary, unless
// the item showed the same text already. Codex prints both only once
// complete.
func (p *Parser) renderMessage(it item, out *adapter.Emitter) contract.RawReason {
	var text string
	if !it.field("text", &text) {
		return contract.RawUnsupported
	}
	if p.items.get(it.id).sameContent(digestOf(it.kind, text)) {
		return ""
	}
	kind := contract.MessageText
	if it.kind == itemReasoning {
		kind = contract.MessageReasoning
	}
	var cut adapter.Truncation
	m := &contract.Message{
		Role:      contract.RoleAssistant,
		Type:      kind,
		Text:      cut.Text("text", text, contract.MaxTextBytes),
		MessageID: cut.Text("messageId", it.id, contract.MaxLabelBytes),
	}
	out.Emit(contract.Event{Kind: contract.KindMessage, Message: m, Truncated: cut.Fields()})
	return ""
}

// renderErrorItem renders a non-fatal error: a warning, a deprecation
// notice, a model reroute.
func (p *Parser) renderErrorItem(it item, out *adapter.Emitter) contract.RawReason {
	var message string
	if !it.field("message", &message) {
		return contract.RawUnsupported
	}
	if p.items.get(it.id).sameContent(digestOf(it.kind, message)) {
		return ""
	}
	var cut adapter.Truncation
	info := &contract.ErrorInfo{Message: cut.Text("message", message, contract.MaxTextBytes)}
	out.Emit(contract.Event{Kind: contract.KindError, Error: info, Truncated: cut.Fields()})
	return ""
}
