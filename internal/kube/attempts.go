package kube

import (
	"cmp"
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/sperano/lucarne/contract"
)

// Attempts lists the attempts of run's agent container that Kubernetes still
// knows about, oldest first. When there are more than
// contract.MaxAttemptsPerRun, only the newest are kept. A readable attempt
// has pending coverage, because its log has not been read yet. An instance
// whose log is gone is expired. A deleted pod is not listed.
func (s *Source) Attempts(ctx context.Context, run Run) ([]contract.Attempt, error) {
	if err := s.readable(run.Key); err != nil {
		return nil, err
	}
	pods, err := s.ownedPods(ctx, run)
	if err != nil {
		return nil, err
	}
	attempts := []contract.Attempt{}
	for i := range pods {
		attempts = append(attempts, podAttempts(&pods[i], runnerOf(run))...)
	}
	if extra := len(attempts) - contract.MaxAttemptsPerRun; extra > 0 {
		attempts = attempts[extra:]
	}
	return attempts, nil
}

// sortPods orders pods by creation, then by name.
func sortPods(pods []corev1.Pod) {
	slices.SortFunc(pods, func(a, b corev1.Pod) int {
		if c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

func runnerOf(run Run) contract.Runner {
	if run.Runner == "" {
		return contract.Runner{Kind: contract.RunnerUnknown}
	}
	return contract.Runner{Kind: run.Runner}
}

// podAttempts lists the instances of pod's agent container, by restart. A
// pod without that container is not a Kelos agent pod and has none. A
// container that never started in a pod that is finished never will, so it
// has none either.
func podAttempts(pod *corev1.Pod, runner contract.Runner) []contract.Attempt {
	if !hasAgentContainer(pod) {
		return nil
	}
	status := agentStatus(pod)
	if status == nil {
		if podFinished(pod) {
			return nil
		}
		reason := "pod " + string(pod.Status.Phase) + ": no container status yet"
		return []contract.Attempt{newAttempt(pod, 0, runner, contract.ContainerState{State: contract.ContainerUnknown}, pending(reason))}
	}
	restarts := int64(status.RestartCount)
	if backingOff(status) {
		attempts := olderAttempts(pod, runner, restarts, restarts)
		return append(attempts, newAttempt(pod, restarts, runner, containerState(status.LastTerminationState), contract.Coverage{State: contract.CoveragePending}))
	}
	attempts := olderAttempts(pod, runner, restarts, restarts-1)
	if restarts > 0 {
		attempts = append(attempts, previousAttempt(pod, status, runner))
	}
	if current, ok := currentAttempt(pod, status, runner); ok {
		attempts = append(attempts, current)
	}
	return attempts
}

// olderAttempts lists the instances of pod's agent container before the
// restart numbered readable, whose logs are gone, at most
// contract.MaxAttemptsPerRun back from the restart count.
func olderAttempts(pod *corev1.Pod, runner contract.Runner, restarts, readable int64) []contract.Attempt {
	var attempts []contract.Attempt
	for r := max(0, restarts-contract.MaxAttemptsPerRun); r < readable; r++ {
		attempts = append(attempts, newAttempt(pod, r, runner, contract.ContainerState{State: contract.ContainerUnknown}, expired(ReasonOlderRestart)))
	}
	return attempts
}

// backingOff reports whether the instance status counts exited and waits to
// be restarted, as in CrashLoopBackOff. The kubelet then keeps that
// instance's container ID on the status and describes it in
// LastTerminationState, and serves its log for the current and the previous
// instance alike: the instance before it can no longer be read.
func backingOff(status *corev1.ContainerStatus) bool {
	last := status.LastTerminationState.Terminated
	return status.State.Waiting != nil && last != nil && last.ContainerID != "" && last.ContainerID == status.ContainerID
}

// previousAttempt is the instance before the current one, readable while
// Kubernetes still reports how it terminated.
func previousAttempt(pod *corev1.Pod, status *corev1.ContainerStatus, runner contract.Runner) contract.Attempt {
	restart := int64(status.RestartCount) - 1
	if status.LastTerminationState.Terminated == nil {
		return newAttempt(pod, restart, runner, contract.ContainerState{State: contract.ContainerUnknown}, expired(ReasonOlderRestart))
	}
	return newAttempt(pod, restart, runner, containerState(status.LastTerminationState), contract.Coverage{State: contract.CoveragePending})
}

// currentAttempt is the instance the status describes. It is not listed when
// the instance never started and its pod is finished.
func currentAttempt(pod *corev1.Pod, status *corev1.ContainerStatus, runner contract.Runner) (contract.Attempt, bool) {
	state := containerState(status.State)
	coverage := contract.Coverage{State: contract.CoveragePending}
	if waiting := status.State.Waiting; waiting != nil || state.State == contract.ContainerUnknown {
		if podFinished(pod) {
			return contract.Attempt{}, false
		}
		coverage = pending(ReasonNotStarted)
		if waiting != nil && waiting.Reason != "" {
			coverage = pending(ReasonNotStarted + ": " + waiting.Reason)
		}
	}
	return newAttempt(pod, int64(status.RestartCount), runner, state, coverage), true
}

func newAttempt(pod *corev1.Pod, restart int64, runner contract.Runner, state contract.ContainerState, coverage contract.Coverage) contract.Attempt {
	key := contract.AttemptKey{PodUID: string(pod.UID), Container: AgentContainer, Restart: restart}
	coverage.Reason = cutLabel(coverage.Reason)
	return contract.Attempt{ID: key.ID(), Key: key, PodName: pod.Name, Runner: runner, Container: state, Coverage: coverage}
}

func hasAgentContainer(pod *corev1.Pod) bool {
	return slices.ContainsFunc(pod.Spec.Containers, func(c corev1.Container) bool { return c.Name == AgentContainer })
}

// agentStatus is the status of pod's agent container, nil before the
// kubelet reported one. Init containers have their own list and are
// never read.
func agentStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == AgentContainer {
			return &pod.Status.ContainerStatuses[i]
		}
	}
	return nil
}

func podFinished(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

// containerState is the contract's view of a Kubernetes container state.
func containerState(s corev1.ContainerState) contract.ContainerState {
	switch {
	case s.Terminated != nil:
		code := int(s.Terminated.ExitCode)
		return contract.ContainerState{
			State:      contract.ContainerTerminated,
			ExitCode:   &code,
			Reason:     cutLabel(s.Terminated.Reason),
			StartedAt:  timestamp(&s.Terminated.StartedAt),
			FinishedAt: timestamp(&s.Terminated.FinishedAt),
		}
	case s.Running != nil:
		return contract.ContainerState{State: contract.ContainerRunning, StartedAt: timestamp(&s.Running.StartedAt)}
	case s.Waiting != nil:
		return contract.ContainerState{State: contract.ContainerWaiting, Reason: cutLabel(s.Waiting.Reason)}
	default:
		return contract.ContainerState{State: contract.ContainerUnknown}
	}
}
