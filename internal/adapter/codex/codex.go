package codex

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Line types of codex exec --json (exec_events.rs ThreadEvent).
const (
	typeThreadStarted = "thread.started"
	typeTurnStarted   = "turn.started"
	typeTurnCompleted = "turn.completed"
	typeTurnFailed    = "turn.failed"
	typeItemStarted   = "item.started"
	typeItemUpdated   = "item.updated"
	typeItemCompleted = "item.completed"
	typeError         = "error"
)

// Parser normalizes codex exec --json lines. It is an adapter.Runner; it
// keeps what it last rendered of each recent item, so that an item's
// started, updated and completed lines render each change once.
type Parser struct {
	items *seenItems
}

// New returns a Parser for one attempt's log.
func New() *Parser { return &Parser{items: newSeenItems()} }

// threadLine holds the fields of every line type the parser renders.
type threadLine struct {
	Type     string          `json:"type"`
	ThreadID *string         `json:"thread_id"`
	Usage    *tokenUsage     `json:"usage"`
	Error    *errorPayload   `json:"error"`
	Message  *string         `json:"message"`
	Item     json.RawMessage `json:"item"`
}

// errorPayload is turn.failed's error.
type errorPayload struct {
	Message *string `json:"message"`
}

// Line implements adapter.Runner.
func (p *Parser) Line(text []byte, out *adapter.Emitter) {
	var head struct {
		Type string `json:"type"`
	}
	if !adapter.DecodeObject(text, out, &head) {
		return
	}
	// Codex writes no time into its lines.
	out.SetSource(head.Type, nil)
	if reason := p.render(head.Type, text, out); reason != "" {
		out.RawLine(reason)
	}
}

// render adds the events of a decoded line, or nothing when the line only
// repeats what its item already showed. It returns why the line was not
// rendered, empty when it was; it adds nothing when it was not.
func (p *Parser) render(lineType string, text []byte, out *adapter.Emitter) contract.RawReason {
	if !knownType(lineType) {
		return contract.RawUnknownType
	}
	var line threadLine
	if json.Unmarshal(text, &line) != nil {
		return contract.RawUnsupported
	}
	switch lineType {
	case typeItemStarted, typeItemUpdated, typeItemCompleted:
		return p.renderItem(lineType == typeItemCompleted, line.Item, out)
	case typeError:
		return renderError(line.Message, out)
	}
	return renderTurn(line, out)
}

func knownType(lineType string) bool {
	switch lineType {
	case typeThreadStarted, typeTurnStarted, typeTurnCompleted, typeTurnFailed,
		typeItemStarted, typeItemUpdated, typeItemCompleted, typeError:
		return true
	}
	return false
}
