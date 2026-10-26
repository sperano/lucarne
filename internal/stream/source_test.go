package stream

import (
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

func TestRunnerWithoutAdapterKeepsLinesRaw(t *testing.T) {
	t.Parallel()
	tests := map[string]contract.RawReason{
		`{"type":"gemini"}`: contract.RawUnknownType,
		"plain text":        contract.RawText,
		`{"cut":`:           contract.RawMalformedJSON,
	}
	for text, want := range tests {
		n := adapter.New(attemptID, contract.LogOrigin([]byte(text)), newRunner(contract.RunnerUnknown))
		evs := n.Line(adapter.Line{Number: 1, Text: []byte(text), Bytes: int64(len(text))})
		if len(evs) != 1 || evs[0].Raw == nil || evs[0].Raw.Reason != want || evs[0].Raw.Text != text {
			t.Errorf("%q gave %+v, want one raw %s", text, evs, want)
		}
	}
}

func TestEveryRunnerKindHasAParser(t *testing.T) {
	t.Parallel()
	for _, kind := range []contract.RunnerKind{contract.RunnerClaudeCode, contract.RunnerCodex, contract.RunnerOpenCode} {
		if _, raw := newRunner(kind).(rawRunner); raw {
			t.Errorf("%s has no adapter", kind)
		}
	}
}
