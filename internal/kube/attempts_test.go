package kube

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sperano/lucarne/contract"
)

func attemptsOf(t *testing.T, objects ...runtime.Object) []contract.Attempt {
	t.Helper()
	c := newCluster(t, []*unstructured.Unstructured{newTask(testNamespace, taskName, taskUID, taskRunning, "claude-code")}, objects...)
	attempts, err := c.src.Attempts(context.Background(), c.run(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range attempts {
		update := contract.AttemptUpdate{Seq: 1, Attempt: a}
		if err := contract.Validate(contract.DefAttemptUpdate, mustJSON(t, update)); err != nil {
			t.Errorf("attempt %s does not validate: %v", a.ID, err)
		}
	}
	c.assertReadOnly(t)
	return attempts
}

func TestAttemptsWithoutJob(t *testing.T) {
	t.Parallel()
	if got := attemptsOf(t); len(got) != 0 {
		t.Errorf("attempts of a Task without a Job = %+v", got)
	}
}

// TestAttemptsNameReuse proves the pods of a Job another Task controls,
// found by the same name and label, are never a run's.
func TestAttemptsNameReuse(t *testing.T) {
	t.Parallel()
	got := attemptsOf(t,
		newJob(taskName, jobUID, reusedUID),
		newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
	if len(got) != 0 {
		t.Errorf("attempts through another Task's Job = %+v", got)
	}
}

// TestAttemptsPodOfAnotherJob proves a pod with the Task's label whose
// controller is another Job is left out.
func TestAttemptsPodOfAnotherJob(t *testing.T) {
	t.Parallel()
	got := attemptsOf(t,
		newJob(taskName, jobUID, taskUID),
		newPod(podName, podUID, reusedUID, 0, corev1.PodRunning, running()),
		newPod(retryPodName, retryPodUID, jobUID, time.Minute, corev1.PodRunning, running()))
	if len(got) != 1 || got[0].Key.PodUID != retryPodUID {
		t.Errorf("attempts = %+v, want only the Job's pod", got)
	}
}

func TestAttemptsPending(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		status *corev1.ContainerStatus
		state  contract.ContainerStateKind
		reason string
	}{
		"no status":       {nil, contract.ContainerUnknown, "pod Pending: no container status yet"},
		"init containers": {waiting("PodInitializing"), contract.ContainerWaiting, ReasonNotStarted + ": PodInitializing"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := attemptsOf(t, newJob(taskName, jobUID, taskUID), newPod(podName, podUID, jobUID, 0, corev1.PodPending, tc.status))
			if len(got) != 1 {
				t.Fatalf("attempts = %+v", got)
			}
			a := got[0]
			if a.ID != podUID+".kelos-agent.0" || a.Container.State != tc.state || a.Coverage.State != contract.CoveragePending || a.Coverage.Reason != tc.reason {
				t.Errorf("attempt = %+v", a)
			}
		})
	}
}

// TestAttemptsRetriedJob proves both pods of a retried Job are listed, the
// failed one first, each by its own UID.
func TestAttemptsRetriedJob(t *testing.T) {
	t.Parallel()
	got := attemptsOf(t,
		newJob(taskName, jobUID, taskUID),
		newPod(retryPodName, retryPodUID, jobUID, time.Minute, corev1.PodRunning, running()),
		newPod(podName, podUID, jobUID, 0, corev1.PodFailed, terminated(1)))
	if len(got) != 2 {
		t.Fatalf("attempts = %+v", got)
	}
	first, second := got[0], got[1]
	if first.Key.PodUID != podUID || first.PodName != podName || first.Container.State != contract.ContainerTerminated || *first.Container.ExitCode != 1 {
		t.Errorf("first = %+v", first)
	}
	if second.Key.PodUID != retryPodUID || second.Container.State != contract.ContainerRunning || second.Runner.Kind != contract.RunnerClaudeCode {
		t.Errorf("second = %+v", second)
	}
	for _, a := range got {
		if a.Coverage.State != contract.CoveragePending {
			t.Errorf("%s coverage = %+v, want pending until read", a.ID, a.Coverage)
		}
	}
}

// TestAttemptsRestarts proves only the current and previous instance are
// readable and older ones are expired.
func TestAttemptsRestarts(t *testing.T) {
	t.Parallel()
	status := running()
	status.RestartCount = 3
	status.LastTerminationState = terminatedState(137)
	got := attemptsOf(t, newJob(taskName, jobUID, taskUID), newPod(podName, podUID, jobUID, 0, corev1.PodRunning, status))
	wantCoverage := []contract.CoverageState{contract.CoverageExpired, contract.CoverageExpired, contract.CoveragePending, contract.CoveragePending}
	wantState := []contract.ContainerStateKind{contract.ContainerUnknown, contract.ContainerUnknown, contract.ContainerTerminated, contract.ContainerRunning}
	if len(got) != len(wantCoverage) {
		t.Fatalf("attempts = %+v", got)
	}
	for i, a := range got {
		if a.Key.Restart != int64(i) || a.Coverage.State != wantCoverage[i] || a.Container.State != wantState[i] {
			t.Errorf("attempt %d = %+v", i, a)
		}
	}
	if got[0].Coverage.Reason != ReasonOlderRestart {
		t.Errorf("expired reason = %q", got[0].Coverage.Reason)
	}
}

// TestAttemptsBackOff proves that while the container backs off, the
// instance that exited is listed as terminated and readable, and the one
// before it, whose log the kubelet no longer serves, as expired.
func TestAttemptsBackOff(t *testing.T) {
	t.Parallel()
	const restarts, exitCode = 2, 137
	got := attemptsOf(t, newJob(taskName, jobUID, taskUID), newPod(podName, podUID, jobUID, 0, corev1.PodRunning, crashLooping(restarts, exitCode)))
	if len(got) != restarts+1 {
		t.Fatalf("attempts = %+v", got)
	}
	for _, a := range got[:restarts] {
		if a.Coverage.State != contract.CoverageExpired || a.Coverage.Reason != ReasonOlderRestart || a.Container.State != contract.ContainerUnknown {
			t.Errorf("older attempt = %+v", a)
		}
	}
	last := got[restarts]
	if last.Key.Restart != restarts || last.Container.State != contract.ContainerTerminated || *last.Container.ExitCode != exitCode || last.Coverage.State != contract.CoveragePending || last.Coverage.Reason != "" {
		t.Errorf("exited attempt = %+v", last)
	}
}

func TestAttemptsBounded(t *testing.T) {
	t.Parallel()
	status := running()
	status.RestartCount = 3 * contract.MaxAttemptsPerRun
	got := attemptsOf(t, newJob(taskName, jobUID, taskUID), newPod(podName, podUID, jobUID, 0, corev1.PodRunning, status))
	if len(got) != contract.MaxAttemptsPerRun {
		t.Fatalf("%d attempts, want %d", len(got), contract.MaxAttemptsPerRun)
	}
	if last := got[len(got)-1]; last.Key.Restart != int64(status.RestartCount) {
		t.Errorf("last attempt = %+v, want the current instance", last)
	}
}

// TestAttemptsNeverStarted proves a finished pod whose agent container never
// ran, as when an init container failed, lists no attempt: there is no log.
func TestAttemptsNeverStarted(t *testing.T) {
	t.Parallel()
	got := attemptsOf(t, newJob(taskName, jobUID, taskUID), newPod(podName, podUID, jobUID, 0, corev1.PodFailed, waiting("PodInitializing")))
	if len(got) != 0 {
		t.Errorf("attempts = %+v", got)
	}
}

// TestAttemptsOnlyAgentContainer proves user containers next to the agent
// are never attempts, and a pod without the agent container has none.
func TestAttemptsOnlyAgentContainer(t *testing.T) {
	t.Parallel()
	withExtra := newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running())
	withExtra.Spec.Containers = append(withExtra.Spec.Containers, corev1.Container{Name: "sidecar"})
	withExtra.Status.ContainerStatuses = append(withExtra.Status.ContainerStatuses, corev1.ContainerStatus{Name: "sidecar"})
	noAgent := newPod(retryPodName, retryPodUID, jobUID, time.Minute, corev1.PodRunning, nil)
	noAgent.Spec.Containers = []corev1.Container{{Name: "sidecar"}}
	got := attemptsOf(t, newJob(taskName, jobUID, taskUID), withExtra, noAgent)
	if len(got) != 1 || got[0].Key.Container != AgentContainer || got[0].Key.PodUID != podUID {
		t.Errorf("attempts = %+v", got)
	}
}
