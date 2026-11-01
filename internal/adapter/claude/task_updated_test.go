package claude

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestTaskUpdatedReportsTheProgressOfItsCall(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_1","description":"make check"}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"paused","is_backgrounded":true}}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"failed","end_time":1793413121362,"error":"exit status 2"}}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"completed"}}`,
	)
	if len(events) != 4 {
		t.Fatalf("events = %s", dump(events))
	}
	paused := events[1].ToolProgress
	if paused == nil || paused.CallID != "toolu_1" || paused.Phase != contract.ToolProgressRunning ||
		paused.Background == nil || !*paused.Background || paused.ElapsedMs != nil {
		t.Errorf("paused = %+v, want toolu_1 running in the background", paused)
	}
	failed := events[2].ToolProgress
	if failed == nil || failed.Phase != contract.ToolProgressFailed || failed.Detail != "exit status 2" {
		t.Errorf("failed = %+v, want failed with the patch's error", failed)
	}
	// The failure ended the task: a later update names a task no longer known.
	if life := events[3].Lifecycle; life == nil || life.Phase != contract.LifecycleRunnerNotice {
		t.Errorf("update after the end = %s, want a runner notice", dump(events[3:]))
	}
}

func TestTaskUpdatedDetail(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_1"}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"running","description":"step 2"}}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"failed","error":"exit 2","description":"make check"}}`,
	)
	if len(events) != 3 {
		t.Fatalf("events = %s", dump(events))
	}
	// The description is the detail, unless the patch has an error.
	for i, want := range []string{"step 2", "exit 2"} {
		if p := events[i+1].ToolProgress; p == nil || p.Detail != want {
			t.Errorf("update %d = %s, want detail %q", i+1, dump(events[i+1:i+2]), want)
		}
	}
}

func TestTaskProgressRecordsItsCall(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"system","subtype":"task_progress","task_id":"task-9","tool_use_id":"toolu_9","description":"step"}`,
		`{"type":"system","subtype":"task_updated","task_id":"task-9","patch":{"status":"running"}}`,
	)
	if len(events) != 2 {
		t.Fatalf("events = %s", dump(events))
	}
	if p := events[1].ToolProgress; p == nil || p.CallID != "toolu_9" || p.Phase != contract.ToolProgressRunning {
		t.Errorf("update = %s, want toolu_9 running", dump(events[1:]))
	}
}

func TestTaskUpdatedWithoutItsCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		lines   []string
		detail  string
		isError bool
	}{
		{
			"task from before the log",
			[]string{`{"type":"system","subtype":"task_updated","task_id":"task-0","patch":{"status":"completed","end_time":1}}`},
			"completed", false,
		},
		{
			"failed task from before the log",
			[]string{`{"type":"system","subtype":"task_updated","task_id":"task-0","patch":{"status":"failed","error":"boom"}}`},
			"failed\n\nboom", true,
		},
		{
			"patch without status",
			[]string{
				`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_1"}`,
				`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"description":"renamed"}}`,
			},
			"renamed", false,
		},
		{
			"task ended by its notification",
			[]string{
				`{"type":"system","subtype":"task_started","task_id":"task-1","tool_use_id":"toolu_1"}`,
				`{"type":"system","subtype":"task_notification","task_id":"task-1","tool_use_id":"toolu_1","status":"completed"}`,
				`{"type":"system","subtype":"task_updated","task_id":"task-1","patch":{"status":"completed"}}`,
			},
			"completed", false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			events := normalize(t, c.lines...)
			life := events[len(events)-1].Lifecycle
			if life == nil || life.Phase != contract.LifecycleRunnerNotice || life.Subtype != subtypeTaskUpdated ||
				life.Detail == nil || *life.Detail != c.detail || (life.IsError != nil) != c.isError {
				t.Errorf("events = %s, want a task_updated notice with detail %q", dump(events), c.detail)
			}
		})
	}
}

func TestTaskUpdatedWithoutPatchIsRaw(t *testing.T) {
	t.Parallel()
	line := `{"type":"system","subtype":"task_updated","task_id":"task-1"}`
	if raw := only(t, normalize(t, line), contract.KindRaw); raw.Raw.Reason != contract.RawUnsupported {
		t.Errorf("raw = %+v, want unsupported", raw.Raw)
	}
}

func TestTaskNotificationStoppedIsKilled(t *testing.T) {
	t.Parallel()
	line := `{"type":"system","subtype":"task_notification","tool_use_id":"toolu_1","status":"stopped"}`
	if p := only(t, normalize(t, line), contract.KindToolProgress).ToolProgress; p.Phase != contract.ToolProgressKilled {
		t.Errorf("phase = %s, want killed", p.Phase)
	}
}
