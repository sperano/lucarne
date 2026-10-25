package codex

import (
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

func TestThreadAndTurnLines(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"thread.started","thread_id":"t-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":4,"cache_write_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}`,
		`{"type":"turn.failed","error":{"message":"boom"}}`,
		`{"type":"turn.failed"}`,
	)
	if l := events[0].Lifecycle; l.Phase != contract.LifecycleSessionStarted || l.SessionID != "t-1" || l.Model != "" {
		t.Errorf("thread.started = %+v, want session t-1 and no model", l)
	}
	if events[1].Lifecycle.Phase != contract.LifecycleTurnStarted {
		t.Errorf("turn.started = %+v", events[1].Lifecycle)
	}
	if events[2].Lifecycle.Phase != contract.LifecycleTurnCompleted {
		t.Errorf("turn.completed = %+v", events[2].Lifecycle)
	}
	u := events[3].Usage
	if u.Scope != contract.ScopeSession || *u.InputTokens != 10 || *u.CacheReadTokens != 4 ||
		*u.CacheWriteTokens != 2 || *u.OutputTokens != 3 || *u.ReasoningTokens != 1 || u.CostUSD != "" {
		t.Errorf("usage = %+v", u)
	}
	if d := events[4].Lifecycle.Detail; events[4].Lifecycle.Phase != contract.LifecycleTurnFailed || d == nil || *d != "boom" {
		t.Errorf("turn.failed = %+v", events[4].Lifecycle)
	}
	if l := events[5].Lifecycle; l.Phase != contract.LifecycleTurnFailed || l.Detail != nil {
		t.Errorf("turn.failed without error = %+v, want no detail", l)
	}
}

func TestRenderTurnUnknownType(t *testing.T) {
	t.Parallel()
	var out adapter.Emitter
	if reason := renderTurn(threadLine{Type: "turn.paused"}, &out); reason != contract.RawUnsupported || out.Len() != 0 {
		t.Errorf("unknown type gave %q and %d events, want unsupported and none", reason, out.Len())
	}
}

func TestUsageNotReported(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		`{"type":"turn.completed"}`,
		`{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}`,
		`{"type":"turn.completed","usage":{"input_tokens":-1}}`,
	} {
		events := normalize(t, line)
		if len(events) != 1 || events[0].Kind != contract.KindLifecycle {
			t.Errorf("%s gave %s, want the lifecycle event alone", line, dump(events))
		}
	}
}

func TestUsageDropsInvalidCounters(t *testing.T) {
	t.Parallel()
	events := normalize(t, `{"type":"turn.completed","usage":{"input_tokens":9007199254740992,"output_tokens":-3,"cached_input_tokens":5}}`)
	u := only(t, events, contract.KindUsage).Usage
	if u.InputTokens != nil || u.OutputTokens != nil || *u.CacheReadTokens != 5 {
		t.Errorf("usage = %+v, want only cacheReadTokens", u)
	}
}

func TestErrorRetries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		message string
		attempt int
		max     *int
	}{
		{"Reconnecting... 2/5 (stream disconnected before completion: example)", 2, ptr(5)},
		{"Reconnecting... 1/5", 1, ptr(5)},
		{"Reconnecting... 3/0 (example)", 3, nil},
		{"Reconnecting... 0/5 (example)", 0, nil},
		{"Reconnecting... waiting for network (example)", 0, nil},
		{"Reconnecting... 2/5x", 0, nil},
		{"Reconnecting... 999999/999999 (example)", maxRetryCount, ptr(maxRetryCount)},
		{"Reconnecting... 1000000/5 (example)", 0, nil},
		{"unexpected status 401 Unauthorized", 0, nil},
	}
	for _, c := range cases {
		info := only(t, normalize(t, `{"type":"error","message":"`+c.message+`"}`), contract.KindError).Error
		if info.Message != c.message {
			t.Errorf("message = %q, want %q", info.Message, c.message)
		}
		switch {
		case c.attempt == 0 && info.Retry != nil:
			t.Errorf("%q gave retry %+v, want none", c.message, info.Retry)
		case c.attempt != 0 && (info.Retry == nil || info.Retry.Attempt != c.attempt):
			t.Errorf("%q gave retry %+v, want attempt %d", c.message, info.Retry, c.attempt)
		case c.attempt != 0 && !equalPtr(info.Retry.Max, c.max):
			t.Errorf("%q gave max %v, want %v", c.message, info.Retry.Max, c.max)
		}
	}
}

func ptr(n int) *int { return &n }

func equalPtr(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
