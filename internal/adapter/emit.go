package adapter

import (
	"bytes"
	"encoding/json"

	"github.com/sperano/lucarne/contract"
)

// Emitter collects the events one line produces and numbers them: each
// gets the line's event ID with the next part, from 0.
type Emitter struct {
	attempt    string
	origin     string
	line       Line
	sourceType string
	runnerTime *contract.Timestamp
	events     []contract.Event
}

// SetSource records the runner's own type of the line and a time the runner
// wrote into it, nil when it wrote none. Both apply to the events emitted
// after.
func (e *Emitter) SetSource(sourceType string, runnerTime *contract.Timestamp) {
	e.sourceType, _ = Cut(sourceType, contract.MaxLabelBytes)
	e.runnerTime = runnerTime
}

// Emit adds an event whose Kind, payload and Truncated the caller set; the
// emitter sets its ID, attempt and source. Seq is left to the stream.
func (e *Emitter) Emit(ev contract.Event) {
	part := len(e.events)
	ev.ID = contract.EventID(e.attempt, e.origin, e.line.Number, part)
	ev.Attempt = e.attempt
	ev.Source = contract.Source{
		Line:       e.line.Number,
		Part:       part,
		Type:       e.sourceType,
		LogTime:    e.line.LogTime,
		RunnerTime: e.runnerTime,
	}
	e.events = append(e.events, ev)
}

// Len is the number of events added so far.
func (e *Emitter) Len() int { return len(e.events) }

// Raw adds a raw event holding text, the whole line or the part of it no
// adapter rendered, cut to contract.MaxRawBytes.
func (e *Emitter) Raw(reason contract.RawReason, text []byte) {
	e.raw(reason, text, int64(len(text)))
}

// RawLine adds the whole line as a raw event.
func (e *Emitter) RawLine(reason contract.RawReason) {
	e.raw(reason, e.line.Text, e.line.Bytes)
}

func (e *Emitter) raw(reason contract.RawReason, text []byte, size int64) {
	var cut Truncation
	e.Emit(contract.Event{
		Kind: contract.KindRaw,
		Raw: &contract.Raw{
			Reason: reason,
			Text:   cut.Text("text", string(text), contract.MaxRawBytes),
			Bytes:  size,
		},
		Truncated: cut.Fields(),
	})
}

// oversized adds the start of a line longer than contract.MaxLineBytes.
func (e *Emitter) oversized() {
	var cut Truncation
	text, _ := Cut(string(e.line.Text), contract.MaxRawBytes)
	cut.Mark("text")
	e.Emit(contract.Event{
		Kind:      contract.KindRaw,
		Raw:       &contract.Raw{Reason: contract.RawOversized, Text: text, Bytes: e.line.Bytes},
		Truncated: cut.Fields(),
	})
}

// DecodeObject decodes a line holding one JSON object into v. When the line
// holds none, it adds the line as raw and returns false: text when the line
// does not look like JSON, malformed_json when it starts as an object but
// does not parse (a cut line). v must accept any object; a field of the
// wrong type is the caller's to report.
func DecodeObject(text []byte, out *Emitter, v any) bool {
	if !looksLikeObject(text) {
		out.RawLine(contract.RawText)
		return false
	}
	if !json.Valid(text) {
		out.RawLine(contract.RawMalformedJSON)
		return false
	}
	if err := json.Unmarshal(text, v); err != nil {
		out.RawLine(contract.RawUnknownType)
		return false
	}
	return true
}

func looksLikeObject(text []byte) bool {
	trimmed := bytes.TrimLeft(text, " \t\r")
	return len(trimmed) > 0 && trimmed[0] == '{'
}
