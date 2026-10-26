package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sperano/lucarne/contract"
)

func TestNewRejectsConfig(t *testing.T) {
	t.Parallel()
	c := newCluster(t, nil)
	clients := Clients{Dynamic: c.dyn, Kube: c.kube, Logs: c.logs}
	cases := map[string]struct {
		cfg     Config
		clients Clients
	}{
		"no namespace":      {Config{Cluster: testCluster}, clients},
		"cluster not label": {Config{Cluster: "Example", Namespaces: []string{testNamespace}}, clients},
		"namespace invalid": {Config{Cluster: testCluster, Namespaces: []string{"agents/x"}}, clients},
		"no log client":     {Config{Cluster: testCluster, Namespaces: []string{testNamespace}}, Clients{Dynamic: c.dyn, Kube: c.kube}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(tc.cfg, tc.clients); err == nil {
				t.Fatal("New accepted it")
			}
		})
	}
}

// TestRunOutsideAllowlist proves a key of another cluster or namespace is
// not found without a call to the API, even when that namespace has the
// Task.
func TestRunOutsideAllowlist(t *testing.T) {
	t.Parallel()
	c := newCluster(t, []*unstructured.Unstructured{newTask(otherNS, taskName, taskUID, taskRunning, "claude-code")})
	keys := map[string]contract.RunKey{
		"other namespace": {Cluster: testCluster, Namespace: otherNS, TaskUID: taskUID},
		"other cluster":   {Cluster: "elsewhere", Namespace: testNamespace, TaskUID: taskUID},
		"invalid UID":     {Cluster: testCluster, Namespace: testNamespace, TaskUID: "Not-A-UID"},
	}
	// Serial: the check after the loop counts the calls of every subtest.
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			if _, err := c.src.Run(context.Background(), key); !errors.Is(err, ErrRunNotFound) {
				t.Fatalf("Run = %v, want ErrRunNotFound", err)
			}
		})
	}
	if n := c.calls(); n != 0 {
		t.Errorf("source made %d calls for keys outside its allowlist", n)
	}
}

// TestEntryPointsCheckAllowlist proves a Run whose key a caller changed is
// refused by every entry point before any call.
func TestEntryPointsCheckAllowlist(t *testing.T) {
	t.Parallel()
	c := newCluster(t, []*unstructured.Unstructured{newTask(testNamespace, taskName, taskUID, taskRunning, "claude-code")})
	run := c.run(t)
	run.Key.Namespace = otherNS
	before := c.calls()
	ctx := context.Background()
	if _, err := c.src.Refresh(ctx, run); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("Refresh = %v", err)
	}
	if _, err := c.src.Attempts(ctx, run); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("Attempts = %v", err)
	}
	key := contract.AttemptKey{PodUID: podUID, Container: AgentContainer}
	if _, err := c.src.ReadLog(ctx, run, key, ReadOptions{}, &sink{}); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("ReadLog = %v", err)
	}
	if n := c.calls() - before; n != 0 {
		t.Errorf("source made %d calls for a changed key", n)
	}
}

func TestRunFindsTaskByUID(t *testing.T) {
	t.Parallel()
	task := newTask(testNamespace, taskName, taskUID, taskSucceeded, "codex")
	start, end := baseTime.Add(time.Second), baseTime.Add(time.Hour)
	if err := unstructured.SetNestedField(task.Object, start.Format(time.RFC3339), "status", "startTime"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(task.Object, end.Format(time.RFC3339), "status", "completionTime"); err != nil {
		t.Fatal(err)
	}
	other := newTask(testNamespace, "example-codex-2", reusedUID, taskRunning, "codex")
	c := newCluster(t, []*unstructured.Unstructured{other, task})
	run := c.run(t)
	want := contract.Run{
		Key:      contract.RunKey{Cluster: testCluster, Namespace: testNamespace, TaskUID: taskUID},
		TaskName: taskName, Runner: contract.RunnerCodex, Phase: contract.PhaseSucceeded,
	}
	if run.Key != want.Key || run.TaskName != want.TaskName || run.Runner != want.Runner || run.Phase != want.Phase {
		t.Errorf("run = %+v, want %+v", run.Run, want)
	}
	for name, got := range map[string]*contract.Timestamp{"created": run.Created, "started": run.Started, "completed": run.Completed} {
		if got == nil || got.Location() != time.UTC {
			t.Errorf("%s = %v, want a UTC time", name, got)
		}
	}
	if !run.Completed.Equal(end) {
		t.Errorf("completed = %v, want %v", run.Completed, end)
	}
	c.assertReadOnly(t)
}

// TestRunNameReuse proves a link to a Task whose name a later Task took
// never opens the later one.
func TestRunNameReuse(t *testing.T) {
	t.Parallel()
	c := newCluster(t, []*unstructured.Unstructured{newTask(testNamespace, taskName, taskUID, taskRunning, "claude-code")})
	run := c.run(t)
	tasks := c.dyn.Resource(TaskResource).Namespace(testNamespace)
	if err := tasks.Delete(context.Background(), taskName, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(context.Background(), newTask(testNamespace, taskName, reusedUID, taskPending, "claude-code"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	c.dyn.ClearActions()
	if _, err := c.src.Refresh(context.Background(), run); !errors.Is(err, ErrRunDeleted) {
		t.Errorf("Refresh = %v, want ErrRunDeleted", err)
	}
	if _, err := c.src.Run(context.Background(), run.Key); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("Run of the old key = %v, want ErrRunNotFound", err)
	}
	c.assertReadOnly(t)
}

func TestRunPaginates(t *testing.T) {
	t.Parallel()
	c := newCluster(t, nil)
	const token = "page-2"
	var continues []string
	c.dyn.PrependReactor("list", "tasks", func(a k8stesting.Action) (bool, runtime.Object, error) {
		opts := a.(k8stesting.ListActionImpl).ListOptions
		continues = append(continues, opts.Continue)
		if opts.Limit != taskListPageSize {
			t.Errorf("limit = %d", opts.Limit)
		}
		list := &unstructured.UnstructuredList{Object: map[string]any{"apiVersion": "kelos.dev/v1alpha2", "kind": "TaskList"}}
		if opts.Continue == "" {
			list.SetContinue(token)
			list.Items = []unstructured.Unstructured{*newTask(testNamespace, "example-codex-2", reusedUID, taskRunning, "codex")}
		} else {
			list.Items = []unstructured.Unstructured{*newTask(testNamespace, taskName, taskUID, taskRunning, "claude-code")}
		}
		return true, list, nil
	})
	if run := c.run(t); run.TaskName != taskName {
		t.Errorf("run = %q", run.TaskName)
	}
	if len(continues) != 2 || continues[1] != token {
		t.Errorf("continue tokens = %q", continues)
	}
}

func TestRunAPIErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  error
		want error
	}{
		"forbidden":    {apierrors.NewForbidden(schema.GroupResource{Group: "kelos.dev", Resource: "tasks"}, "", errors.New("no")), ErrForbidden},
		"unauthorized": {apierrors.NewUnauthorized("expired token"), ErrForbidden},
		"server error": {apierrors.NewInternalError(errors.New("etcd")), ErrUnavailable},
		"timeout":      {apierrors.NewTimeoutError("slow", 1), ErrUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCluster(t, nil)
			c.dyn.PrependReactor("list", "tasks", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, tc.err
			})
			_, err := c.src.Run(context.Background(), contract.RunKey{Cluster: testCluster, Namespace: testNamespace, TaskUID: taskUID})
			if !errors.Is(err, tc.want) {
				t.Errorf("Run = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRunCanceled(t *testing.T) {
	t.Parallel()
	c := newCluster(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	c.dyn.PrependReactor("list", "tasks", func(k8stesting.Action) (bool, runtime.Object, error) {
		cancel()
		return true, nil, context.Canceled
	})
	_, err := c.src.Run(ctx, contract.RunKey{Cluster: testCluster, Namespace: testNamespace, TaskUID: taskUID})
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnavailable) {
		t.Errorf("Run = %v, want only the context's error", err)
	}
}
