package kubetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
)

// Fabricated identities: the repository is public.
const (
	testCluster   = "example"
	testNamespace = "agents"
	taskName      = "example-claude-1"
	taskUID       = "3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"
	podName       = "example-claude-1-abcde"
	podUID        = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f"
	foreignUID    = "8d0e1f2a-3b4c-4d5e-8f70-8b9c0d1e2f3a"
	testTimeout   = 5 * time.Second
)

// lineSink records the lines and gaps of a read.
type lineSink struct {
	texts []string
	gaps  []contract.GapReason
	// onLine runs after each line.
	onLine func(n int)
}

func (s *lineSink) Line(_ string, l adapter.Line) error {
	s.texts = append(s.texts, string(l.Text))
	if s.onLine != nil {
		s.onLine(len(s.texts))
	}
	return nil
}

func (s *lineSink) Gap(_ string, _ int64, gap contract.Gap) error {
	s.gaps = append(s.gaps, gap.Reason)
	return nil
}

func newRun(t *testing.T) (*Cluster, *kube.Source, kube.Run, *Pod) {
	t.Helper()
	c := New(testCluster)
	r, err := c.AddTask(TaskSpec{Namespace: testNamespace, Name: taskName, UID: taskUID, Agent: string(contract.RunnerClaudeCode)})
	if err != nil {
		t.Fatal(err)
	}
	pod, err := r.AddPod(podName, podUID)
	if err != nil {
		t.Fatal(err)
	}
	src, err := c.Source(testNamespace)
	if err != nil {
		t.Fatal(err)
	}
	run, err := src.Run(context.Background(), r.Key())
	if err != nil {
		t.Fatal(err)
	}
	return c, src, run, pod
}

func attempt(t *testing.T, src *kube.Source, run kube.Run) contract.AttemptKey {
	t.Helper()
	attempts, err := src.Attempts(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts %+v, want one", attempts)
	}
	return attempts[0].Key
}

func TestSourceReadsATerminatedLog(t *testing.T) {
	c, src, run, pod := newRun(t)
	if _, err := c.AddForeignPod(testNamespace, "example-claude-1-zzzzz", foreignUID, taskName); err != nil {
		t.Fatal(err)
	}
	pod.Write("one", "two")
	if err := pod.Terminate(1); err != nil {
		t.Fatal(err)
	}
	if run.Phase != contract.PhaseRunning || run.Runner != contract.RunnerClaudeCode {
		t.Errorf("run %+v", run.Run)
	}
	var sink lineSink
	res, err := src.ReadLog(context.Background(), run, attempt(t, src, run), kube.ReadOptions{Follow: true}, &sink)
	if err != nil {
		t.Fatal(err)
	}
	if res.End != kube.EndComplete || !slices.Equal(sink.texts, []string{"one", "two"}) {
		t.Errorf("read %+v, lines %q", res, sink.texts)
	}
	for _, o := range c.LogOpens() {
		if o.Pod != podName {
			t.Errorf("opened the log of %s", o.Pod)
		}
	}
	if w := c.Writes(); len(w) != 0 {
		t.Errorf("source wrote: %v", w)
	}
}

func TestFollowSurvivesABreakWithoutRepeats(t *testing.T) {
	_, src, run, pod := newRun(t)
	pod.Write("one", "two")
	sink := &lineSink{}
	sink.onLine = func(n int) {
		switch n {
		case 2:
			pod.Break()
			pod.Write("three")
		case 3:
			if err := pod.Terminate(0); err != nil {
				t.Error(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	res, err := src.ReadLog(ctx, run, attempt(t, src, run), kube.ReadOptions{Follow: true}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if res.End != kube.EndComplete || !slices.Equal(sink.texts, []string{"one", "two", "three"}) || len(sink.gaps) != 0 {
		t.Errorf("read %+v, lines %q, gaps %v", res, sink.texts, sink.gaps)
	}
}

func TestRotationDuringABreakIsAGap(t *testing.T) {
	_, src, run, pod := newRun(t)
	pod.Write("one", "two")
	sink := &lineSink{}
	sink.onLine = func(n int) {
		switch n {
		case 2:
			pod.Write("lost")
			pod.Rotate()
			pod.Write("three")
			pod.Break()
		case 3:
			if err := pod.Terminate(0); err != nil {
				t.Error(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := src.ReadLog(ctx, run, attempt(t, src, run), kube.ReadOptions{Follow: true}, sink); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sink.texts, []string{"one", "two", "three"}) {
		t.Errorf("lines %q", sink.texts)
	}
	// The rotated log cannot prove that nothing is missing after the
	// break, nor, once it ends, after the last line: a gap for each.
	if len(sink.gaps) == 0 || slices.ContainsFunc(sink.gaps, func(g contract.GapReason) bool { return g != contract.GapReadInterrupted }) {
		t.Errorf("gaps %v, want read_interrupted only", sink.gaps)
	}
}

func TestDeletedRunIsGone(t *testing.T) {
	c := New(testCluster)
	r, err := c.AddTask(TaskSpec{Namespace: testNamespace, Name: taskName, UID: taskUID, Agent: "codex", Phase: PhaseSucceeded})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.AddPod(podName, podUID); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(); err != nil {
		t.Fatal(err)
	}
	src, err := c.Source(testNamespace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Run(context.Background(), r.Key()); !errors.Is(err, kube.ErrRunNotFound) {
		t.Errorf("run of a deleted task: %v, want ErrRunNotFound", err)
	}
}

func TestFailMakesCallsFail(t *testing.T) {
	c, src, run, _ := newRun(t)
	restore := c.Fail("list", "pods", errors.New("injected"))
	if _, err := src.Attempts(context.Background(), run); err == nil {
		t.Error("attempts listed while pods cannot be")
	}
	restore()
	if _, err := src.Attempts(context.Background(), run); err != nil {
		t.Errorf("attempts after restore: %v", err)
	}
	if ns := c.Namespaces(); len(ns) != 1 || !ns[testNamespace] {
		t.Errorf("namespaces called %v", ns)
	}
}

func TestDerivedUIDsDiffer(t *testing.T) {
	job, foreign := derivedUID(taskUID, "job"), derivedUID(taskUID, "foreign")
	if job == foreign || job == taskUID || len(job) != len(taskUID) {
		t.Errorf("derived %s and %s from %s", job, foreign, taskUID)
	}
}
