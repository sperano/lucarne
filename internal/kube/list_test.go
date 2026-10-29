package kube

import (
	"context"
	"errors"
	"fmt"
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

// createdAt sets a Task's creation time.
func createdAt(task *unstructured.Unstructured, at time.Time) *unstructured.Unstructured {
	task.SetCreationTimestamp(metav1.NewTime(at))
	return task
}

// fabricatedUID is the i-th of a series of valid, distinct Task UIDs.
func fabricatedUID(i int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", i)
}

// TestListRunsNewestFirstInAllowlist proves the list holds the Tasks of
// every allowlisted namespace, newest first, and none of another one.
func TestListRunsNewestFirstInAllowlist(t *testing.T) {
	t.Parallel()
	older := createdAt(newTask(testNamespace, taskName, taskUID, taskSucceeded, "codex"), baseTime)
	newer := createdAt(newTask(testNamespace, "example-claude-2", reusedUID, taskRunning, "claude-code"), baseTime.Add(time.Hour))
	hidden := createdAt(newTask(otherNS, "example-hidden", jobUID, taskRunning, "claude-code"), baseTime.Add(2*time.Hour))
	c := newCluster(t, []*unstructured.Unstructured{older, hidden, newer})
	list, err := c.src.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list.Schema != contract.SchemaVersion || list.OmittedRuns != 0 {
		t.Errorf("list = %+v", list)
	}
	var names []string
	for _, run := range list.Runs {
		names = append(names, run.TaskName)
		if run.Key.Cluster != testCluster || run.Key.Namespace != testNamespace {
			t.Errorf("run %s has key %+v", run.TaskName, run.Key)
		}
	}
	if fmt.Sprint(names) != fmt.Sprint([]string{"example-claude-2", taskName}) {
		t.Errorf("runs = %v, want the newer first and no other namespace's", names)
	}
	if got := list.Runs[1]; got.Key.TaskUID != taskUID || got.Runner != contract.RunnerCodex || got.Phase != contract.PhaseSucceeded {
		t.Errorf("run = %+v", got)
	}
	for _, a := range c.dyn.Actions() {
		if a.GetNamespace() != testNamespace {
			t.Errorf("source listed namespace %q", a.GetNamespace())
		}
	}
	c.assertReadOnly(t)
}

// TestListRunsBounded proves a list past the limit keeps the newest runs
// and counts the others, also when they come in several pages.
func TestListRunsBounded(t *testing.T) {
	t.Parallel()
	const extra = 7
	total := contract.MaxListedRuns*listCompactFactor + extra
	c := newCluster(t, nil)
	c.dyn.PrependReactor("list", "tasks", func(a k8stesting.Action) (bool, runtime.Object, error) {
		opts := a.(k8stesting.ListActionImpl).ListOptions
		start := 0
		if opts.Continue != "" {
			if _, err := fmt.Sscan(opts.Continue, &start); err != nil {
				t.Errorf("continue %q", opts.Continue)
			}
		}
		end := min(start+int(opts.Limit), total)
		list := &unstructured.UnstructuredList{Object: map[string]any{"apiVersion": "kelos.dev/v1alpha2", "kind": "TaskList"}}
		for i := start; i < end; i++ {
			task := newTask(testNamespace, fmt.Sprintf("task-%d", i), fabricatedUID(i), taskSucceeded, "codex")
			list.Items = append(list.Items, *createdAt(task, baseTime.Add(time.Duration(i)*time.Minute)))
		}
		if end < total {
			list.SetContinue(fmt.Sprint(end))
		}
		return true, list, nil
	})
	list, err := c.src.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != contract.MaxListedRuns || list.OmittedRuns != int64(total-contract.MaxListedRuns) {
		t.Fatalf("listed %d, omitted %d", len(list.Runs), list.OmittedRuns)
	}
	if first, last := list.Runs[0].TaskName, list.Runs[len(list.Runs)-1].TaskName; first != fmt.Sprintf("task-%d", total-1) ||
		last != fmt.Sprintf("task-%d", total-contract.MaxListedRuns) {
		t.Errorf("runs from %s to %s, want the newest", first, last)
	}
}

// TestListRunsOrder checks the order of runs created at the same time, and
// of a run without a creation time.
func TestListRunsOrder(t *testing.T) {
	t.Parallel()
	at := &contract.Timestamp{Time: baseTime}
	runs := []contract.Run{
		{Key: contract.RunKey{Namespace: "b"}, TaskName: "x"},
		{Key: contract.RunKey{Namespace: "b", TaskUID: "2"}, TaskName: "a", Created: at},
		{Key: contract.RunKey{Namespace: "b", TaskUID: "1"}, TaskName: "a", Created: at},
		{Key: contract.RunKey{Namespace: "a"}, TaskName: "z", Created: at},
	}
	list := contract.RunList{Runs: runs}
	keepNewest(&list, len(runs))
	var got []string
	for _, run := range list.Runs {
		got = append(got, run.Key.Namespace+"/"+run.TaskName+"/"+run.Key.TaskUID)
	}
	if want := []string{"a/z/", "b/a/1", "b/a/2", "b/x/"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order %v, want %v", got, want)
	}
}

// TestListRunsSkipsUndecodableTask proves one malformed Task does not hide
// the others.
func TestListRunsSkipsUndecodableTask(t *testing.T) {
	t.Parallel()
	broken := newTask(testNamespace, "example-broken", reusedUID, taskRunning, "codex")
	broken.Object["status"] = "not an object"
	c := newCluster(t, []*unstructured.Unstructured{broken, newTask(testNamespace, taskName, taskUID, taskRunning, "codex")})
	list, err := c.src.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 1 || list.Runs[0].TaskName != taskName {
		t.Errorf("runs = %+v", list.Runs)
	}
}

func TestListRunsAPIErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  error
		want error
	}{
		"forbidden":    {apierrors.NewForbidden(schema.GroupResource{Group: "kelos.dev", Resource: "tasks"}, "", errors.New("no")), ErrForbidden},
		"server error": {apierrors.NewInternalError(errors.New("etcd")), ErrUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newCluster(t, nil)
			c.dyn.PrependReactor("list", "tasks", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, tc.err
			})
			if _, err := c.src.ListRuns(context.Background()); !errors.Is(err, tc.want) {
				t.Errorf("ListRuns = %v, want %v", err, tc.want)
			}
		})
	}
}
