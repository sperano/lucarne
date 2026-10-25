package adapter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
)

const (
	testAttempt = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.codex.0"
	testOrigin  = "635a06eb36cc2997"
)

// echoRunner emits every line it gets as raw text, so a test sees which
// lines reached the runner.
type echoRunner struct{}

func (echoRunner) Line(_ []byte, out *Emitter) { out.RawLine(contract.RawText) }

// run feeds lines to a Normalizer and returns its events, End's included,
// each checked against the schema.
func run(t *testing.T, lines ...string) []contract.Event {
	t.Helper()
	n := New(testAttempt, testOrigin, echoRunner{})
	var events []contract.Event
	for i, line := range lines {
		events = append(events, n.Line(Line{Number: int64(i + 1), Text: []byte(line), Bytes: int64(len(line))})...)
	}
	events = append(events, n.End()...)
	for i := range events {
		events[i].Seq = int64(i + 1)
		data, err := json.Marshal(events[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := contract.Validate(contract.DefEvent, data); err != nil {
			t.Errorf("event %d: %v\n%s", i, err, data)
		}
	}
	return events
}

func TestBlankLinesProduceNoEvent(t *testing.T) {
	t.Parallel()
	events := run(t, "", "text", "")
	if len(events) != 1 || events[0].Source.Line != 2 {
		t.Fatalf("events = %+v, want one for line 2", events)
	}
	if want := contract.EventID(testAttempt, testOrigin, 2, 0); events[0].ID != want {
		t.Errorf("ID = %s, want %s", events[0].ID, want)
	}
}

func TestOversizedLineKeepsItsStart(t *testing.T) {
	t.Parallel()
	n := New(testAttempt, testOrigin, echoRunner{})
	held := strings.Repeat("a", contract.MaxLineBytes)
	whole := int64(2 * contract.MaxLineBytes)
	events := n.Line(Line{Number: 1, Text: []byte(held), Bytes: whole})
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	raw := events[0].Raw
	if raw.Reason != contract.RawOversized || len(raw.Text) != contract.MaxRawBytes || raw.Bytes != whole {
		t.Errorf("raw = %s, %d bytes of text, bytes %d", raw.Reason, len(raw.Text), raw.Bytes)
	}
	if len(events[0].Truncated) != 1 || events[0].Truncated[0] != "text" {
		t.Errorf("truncated = %v, want [text]", events[0].Truncated)
	}
}

func TestEmitterSetsSource(t *testing.T) {
	t.Parallel()
	logTime := &contract.Timestamp{Time: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}
	runnerTime := &contract.Timestamp{Time: time.Date(2026, 10, 25, 8, 59, 59, 0, time.UTC)}
	out := &Emitter{attempt: testAttempt, origin: testOrigin, line: Line{Number: 7, LogTime: logTime}}
	out.SetSource("assistant", runnerTime)
	out.Raw(contract.RawUnsupported, []byte("a"))
	out.Raw(contract.RawUnsupported, []byte("bc"))
	if out.Len() != 2 {
		t.Fatalf("Len = %d, want 2", out.Len())
	}
	second := out.events[1]
	want := contract.Source{Line: 7, Part: 1, Type: "assistant", LogTime: logTime, RunnerTime: runnerTime}
	if second.Source != want || second.Attempt != testAttempt || second.Raw.Bytes != 2 {
		t.Errorf("event = %+v, want source %+v and 2 bytes", second, want)
	}
	if second.ID != contract.EventID(testAttempt, testOrigin, 7, 1) {
		t.Errorf("ID = %s", second.ID)
	}
}

func TestDecodeObject(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		ok   bool
		want contract.RawReason
	}{
		{`{"type":"x"}`, true, ""},
		{`  {"type":"x"}`, true, ""},
		{`plain text`, false, contract.RawText},
		{`"a string"`, false, contract.RawText},
		{`{"type":`, false, contract.RawMalformedJSON},
		{`{"type":"x"} trailing`, false, contract.RawMalformedJSON},
		{`{"type":1}`, false, contract.RawUnknownType},
	}
	for _, c := range cases {
		out := &Emitter{attempt: testAttempt, origin: testOrigin, line: Line{Number: 1, Text: []byte(c.line), Bytes: int64(len(c.line))}}
		var v struct {
			Type string `json:"type"`
		}
		if ok := DecodeObject([]byte(c.line), out, &v); ok != c.ok {
			t.Errorf("%q: ok = %v, want %v", c.line, ok, c.ok)
			continue
		}
		if c.ok {
			if out.Len() != 0 {
				t.Errorf("%q: decoded and emitted %d events", c.line, out.Len())
			}
			continue
		}
		if out.Len() != 1 || out.events[0].Raw.Reason != c.want {
			t.Errorf("%q: events = %+v, want one raw %s", c.line, out.events, c.want)
		}
	}
}
