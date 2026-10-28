package claude

import (
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestRateLimitStatus(t *testing.T) {
	t.Parallel()
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1793172600,"rateLimitType":"five_hour","overageStatus":"rejected","unifiedWindows":{"five_hour":{"utilization":0.26}}},"session_id":"session-example"}`
	ev := only(t, normalize(t, line), contract.KindLifecycle)
	life := ev.Lifecycle
	if life.Phase != contract.LifecycleRateLimitStatus || life.Subtype != "allowed" || life.SessionID != "session-example" {
		t.Errorf("rate limit lifecycle = %+v", life)
	}
	wantDetail := `{"status":"allowed","resetsAt":1793172600,"rateLimitType":"five_hour","overageStatus":"rejected","unifiedWindows":{"five_hour":{"utilization":0.26}}}`
	if life.Detail == nil || *life.Detail != wantDetail {
		t.Errorf("rate limit detail = %v, want %s", life.Detail, wantDetail)
	}
}

func TestRateLimitUnexpectedShapeStaysRaw(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		`{"type":"rate_limit_event"}`,
		`{"type":"rate_limit_event","rate_limit_info":[]}`,
		`{"type":"rate_limit_event","rate_limit_info":{"resetsAt":1793172600}}`,
	} {
		raw := only(t, normalize(t, line), contract.KindRaw)
		if raw.Raw.Reason != contract.RawUnsupported {
			t.Errorf("%s: raw reason = %s, want %s", line, raw.Raw.Reason, contract.RawUnsupported)
		}
	}
}

func TestRateLimitFieldsAreBounded(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", contract.MaxTextBytes)
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"` + long + `","note":"` + long + `"},"session_id":"` + long + `"}`
	ev := only(t, normalize(t, line), contract.KindLifecycle)
	if got := ev.Truncated; len(got) != 3 || got[0] != "sessionId" || got[1] != "subtype" || got[2] != "detail" {
		t.Errorf("truncated = %v, want sessionId, subtype and detail", got)
	}
}
