package opencode

import (
	"encoding/json"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Line types run --format json emits (run.ts emit).
const (
	typeStepStart  = "step_start"
	typeStepFinish = "step_finish"
	typeText       = "text"
	typeReasoning  = "reasoning"
	typeToolUse    = "tool_use"
	typeError      = "error"
)

// knownTypes are the line types the parser renders.
var knownTypes = map[string]bool{
	typeStepStart: true, typeStepFinish: true, typeText: true,
	typeReasoning: true, typeToolUse: true, typeError: true,
}

// maxUnixMilli is the last millisecond of year 9999, the latest time the
// contract's timestamps can write.
const maxUnixMilli = 253402300799999

// Parser normalizes OpenCode run --format json lines. It is an
// adapter.Runner and keeps no state: every line is a finished part or an
// error, and a tool part carries its call and its result together.
type Parser struct{}

// New returns a Parser for one attempt's log.
func New() *Parser { return &Parser{} }

// head is what every line carries, decoded first.
type head struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
}

// line is the payload of a line: a part, or a session error.
type line struct {
	SessionID string          `json:"sessionID"`
	Part      *part           `json:"part"`
	Error     json.RawMessage `json:"error"`
}

// Line implements adapter.Runner.
func (p *Parser) Line(text []byte, out *adapter.Emitter) {
	var h head
	if !adapter.DecodeObject(text, out, &h) {
		return
	}
	out.SetSource(h.Type, runnerTime(h.Timestamp))
	before := out.Len()
	if !render(h.Type, text, out) || out.Len() == before {
		out.RawLine(rawReason(h.Type))
	}
}

// render adds the events of a decoded line and reports whether it could:
// the type is known and the line has the shape the parser expects. It adds
// nothing when it cannot, and a line that produced nothing is kept raw.
func render(lineType string, text []byte, out *adapter.Emitter) bool {
	if !knownTypes[lineType] {
		return false
	}
	var l line
	if json.Unmarshal(text, &l) != nil {
		return false
	}
	if lineType == typeError {
		return renderError(l.Error, out)
	}
	if l.Part == nil {
		return false
	}
	switch lineType {
	case typeStepStart:
		return renderStepStart(l, out)
	case typeStepFinish:
		return renderStepFinish(l, out)
	case typeText:
		return renderText(*l.Part, contract.MessageText, partText, out)
	case typeReasoning:
		return renderText(*l.Part, contract.MessageReasoning, partReasoning, out)
	default:
		return renderTool(*l.Part, out)
	}
}

// rawReason says why a line was not rendered: a type the parser does not
// know, or a known one whose shape it did not expect.
func rawReason(lineType string) contract.RawReason {
	if knownTypes[lineType] {
		return contract.RawUnsupported
	}
	return contract.RawUnknownType
}

// runnerTime is the line's timestamp, Date.now() in milliseconds when
// run.ts wrote the line; nil when it is absent or not such a number.
func runnerTime(raw json.RawMessage) *contract.Timestamp {
	var ms json.Number
	if len(raw) == 0 || raw[0] == '"' || json.Unmarshal(raw, &ms) != nil {
		return nil
	}
	n, err := ms.Int64()
	if err != nil || n < 0 || n > maxUnixMilli {
		return nil
	}
	return &contract.Timestamp{Time: time.UnixMilli(n).UTC()}
}

func lifecycle(phase contract.LifecyclePhase) contract.Event {
	return contract.Event{Kind: contract.KindLifecycle, Lifecycle: &contract.Lifecycle{Phase: phase}}
}
