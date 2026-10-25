package adapter

import (
	"strconv"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestSetupMarkers(t *testing.T) {
	t.Parallel()
	events := run(t,
		"",
		"---KELOS_SETUP_COMMAND_START---",
		"Cloning into 'repo'...",
		"---KELOS_SETUP_COMMAND_DONE---",
		"---KELOS_SETUP_COMMAND_FAILED--- exit=128",
		"---KELOS_SETUP_COMMAND_FAILED--- exit=?",
		"---KELOS_SETUP_COMMAND_FAILED---",
		"---KELOS_SETUP_COMMAND_STARTED---",
	)
	if len(events) != 7 {
		t.Fatalf("got %d events, want 7: %+v", len(events), events)
	}
	phases := []contract.LifecyclePhase{
		contract.LifecycleSetupStarted, "", contract.LifecycleSetupCompleted,
		contract.LifecycleSetupFailed, contract.LifecycleSetupFailed, contract.LifecycleSetupFailed, "",
	}
	for i, phase := range phases {
		if phase == "" {
			if events[i].Kind != contract.KindRaw {
				t.Errorf("event %d is %s, want the line raw", i, events[i].Kind)
			}
		} else if events[i].Lifecycle == nil || events[i].Lifecycle.Phase != phase {
			t.Errorf("event %d = %+v, want %s", i, events[i], phase)
		}
	}
	if code := events[3].Lifecycle.ExitCode; code == nil || *code != 128 {
		t.Errorf("exit code = %v, want 128", code)
	}
	if d := events[4].Lifecycle.Detail; events[4].Lifecycle.ExitCode != nil || d == nil || *d != "exit=?" {
		t.Errorf("an unparsable exit gave %+v, want it as the detail", events[4].Lifecycle)
	}
	if l := events[5].Lifecycle; l.ExitCode != nil || l.Detail != nil {
		t.Errorf("a bare failure marker gave %+v, want neither field", l)
	}
}

func TestLimitHit(t *testing.T) {
	t.Parallel()
	payload := `{"status": "rejected", "resetsAt": 1792936800}`
	events := run(t, "---KELOS_LIMIT_HIT--- "+payload)
	l := events[0].Lifecycle
	if l.Phase != contract.LifecycleLimitHit || l.Detail == nil || *l.Detail != payload {
		t.Errorf("limit hit = %+v, want the JSON as written", l)
	}
}

func TestOutputsBlock(t *testing.T) {
	t.Parallel()
	events := run(t,
		"before",
		"---KELOS_OUTPUTS_START---",
		"branch: agent/example-42",
		"degenerate:",
		"kelos-capture: example warning",
		"response: VGhlIGdyZWV0aW5n",
		"",
		"---KELOS_OUTPUTS_END---",
		"after",
	)
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	block := events[2]
	if block.Source.Line != 2 || block.ID != contract.EventID(testAttempt, testOrigin, 2, 0) {
		t.Errorf("the block is at %+v, want its start line", block.Source)
	}
	want := []contract.Output{
		{Key: "branch", Value: "agent/example-42"},
		{Key: "degenerate", Value: ""},
		{Key: "response", Value: "VGhlIGdyZWV0aW5n"},
	}
	if !equalOutputs(block.Lifecycle.Outputs, want) {
		t.Errorf("outputs = %+v, want %+v", block.Lifecycle.Outputs, want)
	}
	if events[1].Source.Line != 5 || events[1].Kind != contract.KindRaw {
		t.Errorf("the capture error inside the block is %+v, want it raw", events[1])
	}
	if events[3].Source.Line != 9 {
		t.Errorf("the line after the block is at %d, want 9", events[3].Source.Line)
	}
}

func TestOutputsBlockStderrInside(t *testing.T) {
	t.Parallel()
	events := run(t, "---KELOS_OUTPUTS_START---", "pr: https://github.com/example/widget/pull/7", "Traceback (most recent call last):", "---KELOS_OUTPUTS_END---")
	if len(events) != 2 || events[0].Raw == nil || events[0].Source.Line != 3 {
		t.Fatalf("events = %+v, want the stderr line raw, then the block", events)
	}
	if events[1].Lifecycle == nil || len(events[1].Lifecycle.Outputs) != 1 {
		t.Errorf("block = %+v, want one output", events[1])
	}
}

func TestUnclosedOutputsBlockEndsWithTheLog(t *testing.T) {
	t.Parallel()
	events := run(t, "---KELOS_OUTPUTS_START---", "branch: main", "---KELOS_OUTPUTS_START---", "commit: 9f2c4e1a")
	if len(events) != 2 {
		t.Fatalf("got %d events, want one per block: %+v", len(events), events)
	}
	for i, wantLine := range []int64{1, 3} {
		if events[i].Source.Line != wantLine || len(events[i].Lifecycle.Outputs) != 1 {
			t.Errorf("block %d = %+v, want one output at line %d", i, events[i], wantLine)
		}
	}
}

func TestOutputsBlockIsBounded(t *testing.T) {
	t.Parallel()
	lines := []string{"---KELOS_OUTPUTS_START---"}
	for i := range contract.MaxOutputs + 1 {
		lines = append(lines, "pr: https://github.com/example/widget/pull/"+strconv.Itoa(i))
	}
	lines = append(lines, "response: "+strings.Repeat("A", contract.MaxTextBytes+1), "---KELOS_OUTPUTS_END---")
	events := run(t, lines...)
	block := events[0]
	if len(block.Lifecycle.Outputs) != contract.MaxOutputs || len(block.Truncated) != 1 || block.Truncated[0] != "outputs" {
		t.Errorf("%d outputs, truncated %v; want %d and [outputs]", len(block.Lifecycle.Outputs), block.Truncated, contract.MaxOutputs)
	}
	short := run(t, "---KELOS_OUTPUTS_START---", "response: "+strings.Repeat("A", contract.MaxTextBytes+1), "---KELOS_OUTPUTS_END---")[0]
	if v := short.Lifecycle.Outputs[0].Value; len(v) != contract.MaxTextBytes || short.Truncated[0] != "outputs" {
		t.Errorf("a long value is %d bytes, truncated %v", len(v), short.Truncated)
	}
}

func equalOutputs(a, b []contract.Output) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
