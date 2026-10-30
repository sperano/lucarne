package claude

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestToolProgress(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"task_started","tool_use_id":"toolu_1","description":"make check","is_backgrounded":true,"task_type":"local_bash"}`,
		`{"type":"tool_progress","parent_tool_use_id":"toolu_1","elapsed_time_seconds":30}`,
		`{"type":"system","subtype":"task_notification","tool_use_id":"toolu_1","status":"completed","summary":"make check"}`,
	)
	if len(events) != 3 {
		t.Fatalf("events = %s", dump(events))
	}
	started := events[0].ToolProgress
	if started.Phase != contract.ToolProgressStarted || started.CallID != "toolu_1" ||
		started.Background == nil || !*started.Background || started.Subtype != "local_bash" ||
		started.Detail != "make check" {
		t.Errorf("started = %+v", started)
	}
	running := events[1].ToolProgress
	if running.Phase != contract.ToolProgressRunning || running.ElapsedMs == nil || *running.ElapsedMs != 30000 {
		t.Errorf("running = %+v", running)
	}
	completed := events[2].ToolProgress
	if completed.Phase != contract.ToolProgressCompleted || completed.Detail != "make check" {
		t.Errorf("completed = %+v", completed)
	}
}

func TestToolProgressUnknownAndInvalidValues(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"tool_progress","parent_tool_use_id":"toolu_1","elapsed_time_seconds":-1}`,
		`{"type":"system","subtype":"task_notification","tool_use_id":"toolu_1","status":"paused"}`,
	)
	if events[0].ToolProgress.ElapsedMs != nil {
		t.Errorf("negative elapsed time = %v", events[0].ToolProgress.ElapsedMs)
	}
	if events[1].ToolProgress.Phase != contract.ToolProgressUnknown {
		t.Errorf("unknown status = %s", events[1].ToolProgress.Phase)
	}
}

func TestSubagentTaskProgress(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"task_progress","task_id":"example-task","run_id":"example-run","tool_use_id":"toolu_1","description":"Running go test ./...","subagent_type":"general-purpose","usage":{"total_tokens":57279,"tool_uses":7,"duration_ms":22991},"last_tool_name":"Bash"}`,
		`{"type":"system","subtype":"task_progress","tool_use_id":"toolu_1","usage":{"duration_ms":-1}}`,
	)
	if len(events) != 2 {
		t.Fatalf("events = %s", dump(events))
	}
	running := events[0].ToolProgress
	if running == nil || running.Phase != contract.ToolProgressRunning || running.CallID != "toolu_1" ||
		running.ElapsedMs == nil || *running.ElapsedMs != 22991 || running.Detail != "Running go test ./..." ||
		running.Subtype != "general-purpose" || running.Background != nil {
		t.Fatalf("running = %+v", running)
	}
	if invalid := events[1].ToolProgress; invalid == nil || invalid.ElapsedMs != nil {
		t.Errorf("invalid duration = %+v", invalid)
	}
}
