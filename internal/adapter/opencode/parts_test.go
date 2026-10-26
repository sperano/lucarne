package opencode

import (
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
)

func TestTextRoles(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"text","part":{"type":"text","messageID":"msg_a","text":"Done."}}`,
		`{"type":"text","part":{"type":"text","messageID":"msg_b","text":"Continue.","synthetic":true}}`,
		`{"type":"reasoning","part":{"type":"reasoning","messageID":"msg_a","text":"Thinking."}}`,
		`{"type":"text","part":{"type":"text","text":""}}`,
	)
	want := []contract.Message{
		{Role: contract.RoleAssistant, Type: contract.MessageText, Text: "Done.", MessageID: "msg_a"},
		{Role: contract.RoleUser, Type: contract.MessageText, Text: "Continue.", MessageID: "msg_b"},
		{Role: contract.RoleAssistant, Type: contract.MessageReasoning, Text: "Thinking.", MessageID: "msg_a"},
		{Role: contract.RoleAssistant, Type: contract.MessageText},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events: %s", len(events), adaptertest.Dump(events))
	}
	for i, ev := range events {
		if ev.Message == nil || *ev.Message != want[i] {
			t.Errorf("event %d = %+v, want message %+v", i, ev.Message, want[i])
		}
	}
}

func TestTextIsBounded(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", contract.MaxTextBytes+1)
	ev := only(t, normalize(t, `{"type":"text","part":{"type":"text","text":"`+text+`"}}`), contract.KindMessage)
	if len(ev.Message.Text) != contract.MaxTextBytes || len(ev.Truncated) != 1 || ev.Truncated[0] != "text" {
		t.Errorf("text of %d bytes, truncated %v", len(ev.Message.Text), ev.Truncated)
	}
}

func TestStepLifecycle(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"step_start","sessionID":"ses_a","part":{"type":"step-start","snapshot":"4b825dc6"}}`,
		`{"type":"step_finish","sessionID":"ses_a","part":{"type":"step-finish","reason":"length"}}`,
	)
	started, completed := events[0].Lifecycle, events[1].Lifecycle
	if len(events) != 2 || started.Phase != contract.LifecycleStepStarted || started.SessionID != "ses_a" {
		t.Fatalf("events = %s", adaptertest.Dump(events))
	}
	if completed.Phase != contract.LifecycleStepCompleted || completed.StopReason != "length" || completed.SessionID != "ses_a" {
		t.Errorf("step_finish without usage = %+v", completed)
	}
}

func TestStepUsageKeepsOnlyValidCounters(t *testing.T) {
	t.Parallel()
	line := `{"type":"step_finish","part":{"type":"step-finish","reason":"stop","cost":1e-7,` +
		`"tokens":{"input":12,"output":-1,"reasoning":2.5,"cache":{"read":3,"write":0}}}}`
	usage := only(t, normalize(t, line), contract.KindUsage).Usage
	if usage.Scope != contract.ScopeStep || usage.CostUSD != "0.0000001" {
		t.Errorf("usage = %+v", usage)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 12 || usage.CacheReadTokens == nil || *usage.CacheReadTokens != 3 ||
		usage.CacheWriteTokens == nil || *usage.CacheWriteTokens != 0 {
		t.Errorf("valid counters = %+v", usage)
	}
	if usage.OutputTokens != nil || usage.ReasoningTokens != nil || usage.TotalTokens != nil {
		t.Errorf("a negative, fractional or absent counter was kept: %+v", usage)
	}
}

func TestStepUsageWithoutCountersIsNotEmitted(t *testing.T) {
	t.Parallel()
	events := normalize(t, `{"type":"step_finish","part":{"type":"step-finish","reason":"stop","cost":-1,"tokens":{}}}`)
	if len(events) != 1 || events[0].Kind != contract.KindLifecycle {
		t.Errorf("events = %s", adaptertest.Dump(events))
	}
}
