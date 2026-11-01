package claude

import (
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestRunnerNotices(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		line     string
		subtype  string
		detail   string
		isError  bool
		exitCode *int
		model    string
	}{
		{
			name:    "hook started",
			line:    `{"type":"system","subtype":"hook_started","hook_id":"h1","hook_name":"PreToolUse:Bash","hook_event":"PreToolUse"}`,
			subtype: "hook_started", detail: "PreToolUse:Bash",
		},
		{
			name:    "hook progress without a name",
			line:    `{"type":"system","subtype":"hook_progress","hook_event":"Stop","output":"","stdout":"checking","stderr":"slow disk"}`,
			subtype: "hook_progress", detail: "Stop\n\nchecking\n\nslow disk",
		},
		{
			name:    "hook response",
			line:    `{"type":"system","subtype":"hook_response","hook_name":"PreToolUse:Bash","output":"combined","stdout":"out-only","stderr":"err-only","exit_code":0,"outcome":"success"}`,
			subtype: "hook_response", detail: "PreToolUse:Bash\n\ncombined", exitCode: intPtr(0),
		},
		{
			name:    "hook response without a combined output",
			line:    `{"type":"system","subtype":"hook_response","hook_name":"PreToolUse:Bash","output":"","stdout":"out-only","stderr":"err-only","exit_code":0,"outcome":"success"}`,
			subtype: "hook_response", detail: "PreToolUse:Bash\n\nout-only\n\nerr-only", exitCode: intPtr(0),
		},
		{
			name:    "hook failed",
			line:    `{"type":"system","subtype":"hook_response","hook_name":"Stop","output":"denied","exit_code":2,"outcome":"error"}`,
			subtype: "hook_response", detail: "Stop · error\n\ndenied", exitCode: intPtr(2), isError: true,
		},
		{
			name:    "status",
			line:    `{"type":"system","subtype":"status","status":"compacting"}`,
			subtype: "status", detail: "compacting",
		},
		{
			name:    "compaction failed",
			line:    `{"type":"system","subtype":"status","status":null,"compact_result":"failed","compact_error":"context too large"}`,
			subtype: "status", detail: "compact failed\n\ncontext too large", isError: true,
		},
		{
			name:    "informational",
			line:    `{"type":"system","subtype":"informational","content":"Background task finished.","level":"info"}`,
			subtype: "informational", detail: "Background task finished.",
		},
		{
			name:    "notification",
			line:    `{"type":"system","subtype":"notification","key":"k","text":"Context is 80% full","priority":"low"}`,
			subtype: "notification", detail: "Context is 80% full",
		},
		{
			name:    "local command output",
			line:    `{"type":"system","subtype":"local_command_output","content":"ran /cost"}`,
			subtype: "local_command_output", detail: "ran /cost",
		},
		{
			name:    "session state changed",
			line:    `{"type":"system","subtype":"session_state_changed","state":"idle"}`,
			subtype: "session_state_changed", detail: "idle",
		},
		{
			name:    "memory recall",
			line:    `{"type":"system","subtype":"memory_recall","mode":"select","memories":[{"path":"a.md","scope":"personal"},{"path":"b.md","scope":"team"}]}`,
			subtype: "memory_recall", detail: "a.md\nb.md",
		},
		{
			name:    "files persisted",
			line:    `{"type":"system","subtype":"files_persisted","files":[{"filename":"a.md","file_id":"f1"}],"failed":[{"filename":"b.log","error":"too large"}]}`,
			subtype: "files_persisted", detail: "a.md\n\nb.log: too large", isError: true,
		},
		{
			name:    "plugin install failed",
			line:    `{"type":"system","subtype":"plugin_install","status":"failed","name":"example","error":"not found"}`,
			subtype: "plugin_install", detail: "example · failed\n\nnot found", isError: true,
		},
		{
			name:    "background tasks",
			line:    `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"t1","task_type":"local_bash","description":"make build"}]}`,
			subtype: "background_tasks_changed", detail: "make build",
		},
		{
			name:    "refusal with a fallback",
			line:    `{"type":"system","subtype":"model_refusal_fallback","original_model":"model-a","fallback_model":"model-b","content":"Retrying with model-b."}`,
			subtype: "model_refusal_fallback", detail: "Retrying with model-b.", isError: true, model: "model-b",
		},
		{
			name:    "refusal without a fallback",
			line:    `{"type":"system","subtype":"model_refusal_no_fallback","original_model":"model-a","content":"Refused."}`,
			subtype: "model_refusal_no_fallback", detail: "Refused.", isError: true, model: "model-a",
		},
		{
			name:    "mirror error",
			line:    `{"type":"system","subtype":"mirror_error","error":"upload failed","key":{"projectKey":"p","sessionId":"s"}}`,
			subtype: "mirror_error", detail: "upload failed", isError: true,
		},
		{
			name:    "permission denied",
			line:    `{"type":"system","subtype":"permission_denied","tool_name":"WebFetch","tool_use_id":"toolu_1","message":"not allowed"}`,
			subtype: "permission_denied", detail: "WebFetch\n\nnot allowed", isError: true,
		},
		{
			name:    "worker shutting down",
			line:    `{"type":"system","subtype":"worker_shutting_down","reason":"idle"}`,
			subtype: "worker_shutting_down", detail: "idle",
		},
		{
			name:    "tool use summary",
			line:    `{"type":"tool_use_summary","summary":"Read three files.","preceding_tool_use_ids":["toolu_1"]}`,
			subtype: "tool_use_summary", detail: "Read three files.",
		},
		{
			name:    "auth status with an error",
			line:    `{"type":"auth_status","isAuthenticating":false,"output":["line 1","line 2"],"error":"expired"}`,
			subtype: "auth_status", detail: "expired\n\nline 1\nline 2", isError: true,
		},
		{
			name:    "auth status without an error",
			line:    `{"type":"auth_status","isAuthenticating":true,"output":["Opening browser"]}`,
			subtype: "auth_status", detail: "Opening browser",
		},
		{
			name:    "notice of the wrong shape",
			line:    `{"type":"system","subtype":"hook_started","hook_name":7}`,
			subtype: "hook_started",
		},
		{
			name:    "permission denied of the wrong shape",
			line:    `{"type":"system","subtype":"permission_denied","tool_name":"WebFetch","message":{"text":"not allowed"}}`,
			subtype: "permission_denied", isError: true,
		},
		{
			name:    "refusal of the wrong shape",
			line:    `{"type":"system","subtype":"model_refusal_fallback","content":["Retrying."]}`,
			subtype: "model_refusal_fallback", isError: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			life := only(t, normalize(t, c.line), contract.KindLifecycle).Lifecycle
			checkNotice(t, life, c.subtype, c.detail, c.isError)
			if !sameInt(life.ExitCode, c.exitCode) || life.Model != c.model {
				t.Errorf("exit code %v, model %q; want %v, %q", life.ExitCode, life.Model, c.exitCode, c.model)
			}
		})
	}
}

func TestNoticeDetailIsBounded(t *testing.T) {
	t.Parallel()
	line := `{"type":"system","subtype":"notification","text":"` + strings.Repeat("a", contract.MaxTextBytes+1) + `"}`
	ev := only(t, normalize(t, line), contract.KindLifecycle)
	if d := ev.Lifecycle.Detail; d == nil || len(*d) > contract.MaxTextBytes {
		t.Errorf("detail is not cut to MaxTextBytes")
	}
	if len(ev.Truncated) != 1 || ev.Truncated[0] != "detail" {
		t.Errorf("truncated = %v, want [detail]", ev.Truncated)
	}
}

func TestNoticeNameIsBounded(t *testing.T) {
	t.Parallel()
	line := `{"type":"` + strings.Repeat("a", contract.MaxLabelBytes+1) + `"}`
	ev := only(t, normalize(t, line), contract.KindLifecycle)
	if len(ev.Lifecycle.Subtype) > contract.MaxLabelBytes {
		t.Errorf("subtype is %d bytes, want at most MaxLabelBytes", len(ev.Lifecycle.Subtype))
	}
	if len(ev.Truncated) != 1 || ev.Truncated[0] != "subtype" {
		t.Errorf("truncated = %v, want [subtype]", ev.Truncated)
	}
}

func checkNotice(t *testing.T, life *contract.Lifecycle, subtype, detail string, isError bool) {
	t.Helper()
	if life.Phase != contract.LifecycleRunnerNotice || life.Subtype != subtype {
		t.Errorf("lifecycle = %+v, want a %s runner notice", life, subtype)
	}
	got := ""
	if life.Detail != nil {
		got = *life.Detail
	}
	if (life.Detail == nil) != (detail == "") || got != detail {
		t.Errorf("detail = %q, want %q", got, detail)
	}
	if (life.IsError != nil && *life.IsError) != isError || (life.IsError != nil && !*life.IsError) {
		t.Errorf("isError = %v, want %v", life.IsError, isError)
	}
}

func sameInt(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
