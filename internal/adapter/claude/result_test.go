package claude

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestResultErrors(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"api_error_status":529,"errors":["first",{"kind":"second"}]}`,
		`{"type":"result","subtype":"success","is_error":true,"result":"Usage limit reached.","api_error_status":429}`,
		`{"type":"result","subtype":"error_max_turns","is_error":true,"api_error_status":42}`,
	)
	errs := ofKind(events, contract.KindError)
	want := []struct {
		message string
		status  *int
	}{
		{"first", intPtr(529)},
		{`{"kind":"second"}`, intPtr(529)},
		{"Usage limit reached.", intPtr(429)},
	}
	if len(errs) != len(want) {
		t.Fatalf("got %d errors, want %d: %s", len(errs), len(want), dump(events))
	}
	for i, e := range errs {
		if e.Error.Message != want[i].message || *e.Error.Status != *want[i].status {
			t.Errorf("error %d = %+v, want %+v", i, e.Error, want[i])
		}
	}
	if subtype := events[len(events)-1].Lifecycle.Subtype; subtype != "error_max_turns" {
		t.Errorf("the last event is %s, want the session completion of error_max_turns", subtype)
	}
}

func TestResultWithoutCountersHasNoUsage(t *testing.T) {
	t.Parallel()
	events := normalize(t, `{"type":"result","subtype":"success","is_error":false,"total_cost_usd":null}`)
	only(t, events, contract.KindLifecycle)
	if usage := ofKind(events, contract.KindUsage); len(usage) != 0 {
		t.Errorf("a result without counters reported usage: %s", dump(usage))
	}
}

func TestSessionCost(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"0.4182": "0.4182",
		"0":      "0",
		"1e-5":   "0.00001",
		"1.5E+2": "150",
		"-1":     "",
		"1e-30":  "",
	}
	for written, want := range cases {
		t.Run(written, func(t *testing.T) {
			t.Parallel()
			events := normalize(t, `{"type":"result","subtype":"success","duration_ms":1,"total_cost_usd":`+written+`}`)
			if got := only(t, events, contract.KindUsage).Usage.CostUSD; got != want {
				t.Errorf("cost %s = %q, want %q", written, got, want)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
