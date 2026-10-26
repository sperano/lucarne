package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sperano/lucarne/contract"
)

func TestTaskRunner(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		worker, deprecated string
		want               contract.RunnerKind
	}{
		"worker type":           {"opencode", "", contract.RunnerOpenCode},
		"deprecated type":       {"", "codex", contract.RunnerCodex},
		"worker wins":           {"claude-code", "codex", contract.RunnerClaudeCode},
		"agent without adapter": {"gemini", "", contract.RunnerUnknown},
		"none":                  {"", "", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			obj := newTask(testNamespace, taskName, taskUID, taskRunning, tc.worker)
			if err := unstructured.SetNestedField(obj.Object, tc.deprecated, "spec", "type"); err != nil {
				t.Fatal(err)
			}
			task, err := decodeTask(obj)
			if err != nil {
				t.Fatal(err)
			}
			if got := task.runner(); got != tc.want {
				t.Errorf("runner = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTaskPhase(t *testing.T) {
	t.Parallel()
	cases := map[string]contract.RunPhase{
		taskPending: contract.PhasePending, taskWaiting: contract.PhaseWaiting, taskRunning: contract.PhaseRunning,
		taskSucceeded: contract.PhaseSucceeded, taskFailed: contract.PhaseFailed, "": contract.PhaseUnknown, "Paused": contract.PhaseUnknown,
	}
	for phase, want := range cases {
		task, err := decodeTask(newTask(testNamespace, taskName, taskUID, phase, "codex"))
		if err != nil {
			t.Fatal(err)
		}
		if got := task.phase(); got != want {
			t.Errorf("phase %q = %q, want %q", phase, got, want)
		}
	}
}

// TestTaskJobName proves a Task whose status names no Job yet is looked up
// under its own name, as Kelos names the Job.
func TestTaskJobName(t *testing.T) {
	t.Parallel()
	obj := newTask(testNamespace, taskName, taskUID, taskPending, "codex")
	unstructured.RemoveNestedField(obj.Object, "status", "jobName")
	task, err := decodeTask(obj)
	if err != nil {
		t.Fatal(err)
	}
	if got := task.jobName(); got != taskName {
		t.Errorf("jobName = %q", got)
	}
}
