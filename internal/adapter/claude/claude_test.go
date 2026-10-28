package claude

import (
	"flag"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
)

var update = flag.Bool("update", false, "rewrite the golden files from the adapter's output")

// testAttempt is the attempt every test log belongs to.
const testAttempt = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0"

// runnerName is the runner the fixtures under test belong to.
const runnerName = "claude-code"

// normalize runs lines through a Normalizer with a Parser, numbers the
// events as a stream would and checks each against the schema.
func normalize(t *testing.T, lines ...string) []contract.Event {
	t.Helper()
	return adaptertest.Normalize(t, testAttempt, New(), lines...)
}

func only(t *testing.T, events []contract.Event, kind contract.EventKind) contract.Event {
	t.Helper()
	return adaptertest.Only(t, events, kind)
}

func ofKind(events []contract.Event, kind contract.EventKind) []contract.Event {
	return adaptertest.OfKind(events, kind)
}

func dump(events []contract.Event) string { return adaptertest.Dump(events) }

func TestFixturesMatchGolden(t *testing.T) {
	t.Parallel()
	adaptertest.MatchGolden(t, testAttempt, runnerName, func() adapter.Runner { return New() }, *update)
}

func TestLineTypesAndRawReasons(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want contract.RawReason
	}{
		{"text", "(node:17) warning", contract.RawText},
		{"cut object", `{"type":"assistant","mess`, contract.RawMalformedJSON},
		{"array", `[1,2]`, contract.RawText},
		{"unknown type", `{"type":"example_future_type"}`, contract.RawUnknownType},
		{"no type", `{"example":1}`, contract.RawUnknownType},
		{"type not a string", `{"type":7}`, contract.RawUnknownType},
		{"unknown system subtype", `{"type":"system","subtype":"hook_started"}`, contract.RawUnknownType},
		{"thinking token progress", `{"type":"system","subtype":"thinking_tokens","estimated_tokens":1550,"estimated_tokens_delta":200,"session_id":"session-example","uuid":"message-example"}`, contract.RawUnsupported},
		{"partial message", `{"type":"stream_event","event":{"type":"message_start"}}`, contract.RawUnsupported},
		{"assistant without message", `{"type":"assistant"}`, contract.RawUnsupported},
		{"assistant with empty content", `{"type":"assistant","message":{"id":"msg_1","content":[]}}`, contract.RawUnsupported},
		{"user content not an array", `{"type":"user","message":{"content":7}}`, contract.RawUnsupported},
		{"init of the wrong shape", `{"type":"system","subtype":"init","model":7}`, contract.RawUnsupported},
		{"result of the wrong shape", `{"type":"result","is_error":"no"}`, contract.RawUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			raw := only(t, normalize(t, c.line), contract.KindRaw)
			if raw.Raw.Reason != c.want || raw.Raw.Text != c.line || raw.Raw.Bytes != int64(len(c.line)) {
				t.Errorf("raw = %+v, want reason %s holding the line", raw.Raw, c.want)
			}
		})
	}
}

func TestRawTextIsBounded(t *testing.T) {
	t.Parallel()
	line := `{"type":"example_future_type","data":"` + strings.Repeat("é", contract.MaxRawBytes) + `"}`
	raw := only(t, normalize(t, line), contract.KindRaw)
	if len(raw.Raw.Text) > contract.MaxRawBytes || raw.Raw.Bytes != int64(len(line)) {
		t.Errorf("raw text is %d bytes for a %d-byte line, bytes = %d", len(raw.Raw.Text), len(line), raw.Raw.Bytes)
	}
	if len(raw.Truncated) != 1 || raw.Truncated[0] != "text" {
		t.Errorf("truncated = %v, want [text]", raw.Truncated)
	}
}

func TestSystemLines(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"init","session_id":"s-1","model":"claude-opus-5-5","claude_code_version":"2.1.296"}`,
		`{"type":"system","subtype":"api_retry","attempt":0,"max_retries":10,"retry_delay_ms":5,"error_status":600,"error":"unknown"}`,
		`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual", "pre_tokens":5}}`,
		`{"type":"system","subtype":"compact_boundary"}`,
	)
	started := events[0].Lifecycle
	if started.Phase != contract.LifecycleSessionStarted || started.SessionID != "s-1" || started.RunnerVersion != "2.1.296" {
		t.Errorf("init = %+v", started)
	}
	retry := events[1].Error
	if retry.Retry != nil || retry.Status != nil || retry.Message != "unknown" {
		t.Errorf("api_retry with an invalid attempt and status = %+v, want neither", retry)
	}
	if d := events[2].Lifecycle.Detail; d == nil || *d != `{"trigger":"manual","pre_tokens":5}` {
		t.Errorf("compact detail = %v", d)
	}
	if events[3].Lifecycle.Phase != contract.LifecycleCompacted || events[3].Lifecycle.Detail != nil {
		t.Errorf("compact without metadata = %+v", events[3].Lifecycle)
	}
}

func TestRunnerTimeIsTheLineTimestamp(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","timestamp":"2026-10-25T05:00:07.25-04:00","message":{"id":"m","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}}`,
		`{"type":"assistant","timestamp":"yesterday","message":{"id":"m","content":[{"type":"text","text":"c"}]}}`,
	)
	for _, ev := range events[:2] {
		if ev.Source.RunnerTime == nil || ev.Source.RunnerTime.Format("2006-01-02T15:04:05.000Z07:00") != "2026-10-25T09:00:07.250Z" {
			t.Errorf("runnerTime = %v, want the line's timestamp in UTC", ev.Source.RunnerTime)
		}
	}
	if events[2].Source.RunnerTime != nil {
		t.Errorf("an unparseable timestamp gave runnerTime %v", events[2].Source.RunnerTime)
	}
}

func FuzzLine(f *testing.F) {
	adaptertest.FuzzSeeds(f, runnerName)
	f.Fuzz(func(t *testing.T, line string) {
		events := normalize(t, line, line)
		if line != "" && len(events) == 0 {
			t.Errorf("line %q produced no event", line)
		}
	})
}
