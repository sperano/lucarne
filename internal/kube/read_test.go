package kube

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sperano/lucarne/contract"
)

// agentKey is the attempt key of the fabricated pod's agent container.
var agentKey = contract.AttemptKey{PodUID: podUID, Container: AgentContainer}

// readCluster is a running Task with a Job and pods.
func readCluster(t *testing.T, pods ...*corev1.Pod) (*cluster, Run) {
	t.Helper()
	objects := []runtime.Object{newJob(taskName, jobUID, taskUID)}
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	c := newCluster(t, []*unstructured.Unstructured{newTask(testNamespace, taskName, taskUID, taskRunning, "claude-code")}, objects...)
	return c, c.run(t)
}

func (c *cluster) read(t *testing.T, run Run, key contract.AttemptKey, opts ReadOptions) (*sink, ReadResult) {
	t.Helper()
	got := &sink{}
	res, err := c.src.ReadLog(context.Background(), run, key, opts, got)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	c.assertReadOnly(t)
	return got, res
}

// TestReadSnapshotTerminated reads a finished container's whole log: line
// numbers count blank lines, timestamps become LogTime, and the origin is
// the first line as the API returned it.
func TestReadSnapshotTerminated(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0)))
	first := logLine(baseTime, `{"type":"system"}`)
	c.logs.responses = []logResponse{{text: first + logLine(baseTime, "") + strings.TrimSuffix(logLine(baseTime, "last"), "\n")}}
	got, res := c.read(t, run, agentKey, ReadOptions{})
	if res.End != EndComplete || res.Coverage().State != contract.CoverageEnded || res.Lines != 3 || res.Gaps != 0 {
		t.Fatalf("result = %+v", res)
	}
	if want := []string{`{"type":"system"}`, "", "last"}; !slices.Equal(got.texts(), want) {
		t.Errorf("texts = %q, want %q", got.texts(), want)
	}
	for i, l := range got.lines {
		if l.Number != int64(i+1) || l.LogTime == nil || !l.LogTime.Equal(baseTime) {
			t.Errorf("line %d = %+v", i, l)
		}
	}
	if want := contract.LogOrigin([]byte(strings.TrimSuffix(first, "\n"))); res.Origin != want || !got.origins[want] || len(got.origins) != 1 {
		t.Errorf("origin = %s, want %s", res.Origin, want)
	}
	opens := c.logs.opened()
	if len(opens) != 1 || opens[0].Container != AgentContainer || !opens[0].Timestamps || opens[0].Follow || opens[0].Previous || opens[0].SinceTime != nil {
		t.Errorf("opens = %+v", opens)
	}
}

// TestReadSnapshotRunning reads a running container's log as it is: the
// line it is still writing is left for the next read.
func TestReadSnapshotRunning(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
	c.logs.responses = []logResponse{{text: logLine(baseTime, "one") + "2026-10-25T10:00:01Z half"}}
	got, res := c.read(t, run, agentKey, ReadOptions{})
	if res.End != EndCurrent || res.Coverage().State != contract.CoverageFollowing || !slices.Equal(got.texts(), []string{"one"}) {
		t.Errorf("result = %+v, texts %q", res, got.texts())
	}
}

func TestReadPending(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodPending, waiting("PodInitializing")))
	_, res := c.read(t, run, agentKey, ReadOptions{})
	if res.End != EndPending || res.Coverage().State != contract.CoveragePending || len(c.logs.opened()) != 0 {
		t.Errorf("result = %+v, opens %d", res, len(c.logs.opened()))
	}
}

// TestReadFollowWaitsAndResumes follows a container from before it starts:
// the stream breaks and resumes without a gap or a repeated line, then the
// container terminates and the rest of its log is read.
func TestReadFollowWaitsAndResumes(t *testing.T) {
	t.Parallel()
	pending := newPod(podName, podUID, jobUID, 0, corev1.PodPending, waiting("ContainerCreating"))
	started := newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running())
	done := newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0))
	c, run := readCluster(t)
	c.podStates([]*corev1.Pod{pending}, []*corev1.Pod{started}, []*corev1.Pod{started}, []*corev1.Pod{done})
	all := lines(baseTime, 0, 5)
	c.logs.responses = []logResponse{
		{text: lines(baseTime, 0, 3), err: errors.New("connection reset")},
		{text: lines(baseTime, 0, 4)},
		{text: all},
	}
	got, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
	if res.End != EndComplete || res.Gaps != 0 || res.Lines != 5 {
		t.Fatalf("result = %+v, order %q", res, got.order)
	}
	for i, l := range got.lines {
		if want := lines(baseTime, i, 1); string(l.Text) != strings.TrimSuffix(want[strings.IndexByte(want, ' ')+1:], "\n") || l.Number != int64(i+1) {
			t.Errorf("line %d = %d %q", i, l.Number, l.Text)
		}
	}
	opens := c.logs.opened()
	if len(opens) != 3 || !opens[0].Follow || opens[0].SinceTime != nil || !opens[1].Follow || opens[2].Follow {
		t.Fatalf("opens = %+v", opens)
	}
	if !opens[1].SinceTime.Time.Equal(baseTime) {
		t.Errorf("resumed since %v, want the second of the last line", opens[1].SinceTime)
	}
}

// TestReadReconnectLosesLines proves a reconnect that cannot find the last
// lines again reports a gap before the next line instead of guessing.
func TestReadReconnectLosesLines(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t)
	started := newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running())
	c.podStates([]*corev1.Pod{started}, []*corev1.Pod{newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0))})
	c.logs.responses = []logResponse{
		{text: lines(baseTime, 0, 2), err: errors.New("connection reset")},
		{text: lines(baseTime, 3000, 2)},
	}
	got, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
	want := []string{"line 1", "line 2", "gap read_interrupted before 3", "line 3", "line 4"}
	if !slices.Equal(got.order, want) || res.Gaps != 1 || res.End != EndComplete {
		t.Errorf("order = %q, result %+v", got.order, res)
	}
}

// TestReadPodDeletedWhileFollowing proves a pod deleted mid-read ends it as
// expired, with a gap saying the rest is missing.
func TestReadPodDeletedWhileFollowing(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t)
	c.podStates([]*corev1.Pod{newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running())}, nil)
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 2), err: errors.New("stream closed")}}
	got, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
	want := []string{"line 1", "line 2", "gap attempt_expired before 3"}
	if !slices.Equal(got.order, want) || res.End != EndExpired || res.Reason != ReasonPodDeleted || res.Coverage().State != contract.CoverageExpired {
		t.Errorf("order = %q, result %+v", got.order, res)
	}
}

// TestReadTaskReplaced proves a read checks the Task again: once its name
// holds another Task, the attempt is expired and no log is opened.
func TestReadTaskReplaced(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
	replaced := newTask(testNamespace, taskName, reusedUID, taskPending, "claude-code")
	if _, err := c.dyn.Resource(TaskResource).Namespace(testNamespace).Update(context.Background(), replaced, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	c.dyn.ClearActions()
	got, res := c.read(t, run, agentKey, ReadOptions{})
	if res.End != EndExpired || res.Reason != ReasonRunDeleted || len(got.gaps) != 0 || len(c.logs.opened()) != 0 {
		t.Errorf("result = %+v, opens %d", res, len(c.logs.opened()))
	}
}

// TestReadForeignPod proves an attempt key naming a pod that is not the
// run's, even in the same namespace, never opens its log.
func TestReadForeignPod(t *testing.T) {
	t.Parallel()
	foreign := newPod(retryPodName, retryPodUID, reusedUID, 0, corev1.PodRunning, running())
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()), foreign)
	key := contract.AttemptKey{PodUID: retryPodUID, Container: AgentContainer}
	_, res := c.read(t, run, key, ReadOptions{})
	if res.End != EndExpired || len(c.logs.opened()) != 0 {
		t.Errorf("result = %+v, opens %d", res, len(c.logs.opened()))
	}
}

func TestReadAttemptNotFound(t *testing.T) {
	t.Parallel()
	status := running()
	status.RestartCount = 1
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, status))
	keys := map[string]contract.AttemptKey{
		"init container":  {PodUID: podUID, Container: "git-clone"},
		"future restart":  {PodUID: podUID, Container: AgentContainer, Restart: 2},
		"invalid pod UID": {PodUID: "Pod", Container: AgentContainer},
	}
	// Serial: the check after the loop counts the calls of every subtest.
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			_, err := c.src.ReadLog(context.Background(), run, key, ReadOptions{}, &sink{})
			if !errors.Is(err, ErrAttemptNotFound) {
				t.Errorf("ReadLog = %v, want ErrAttemptNotFound", err)
			}
		})
	}
	if len(c.logs.opened()) != 0 {
		t.Errorf("opened %d logs", len(c.logs.opened()))
	}
}

// TestReadRestarts proves the previous instance is read with Previous and
// an older one is expired without a read.
func TestReadRestarts(t *testing.T) {
	t.Parallel()
	status := running()
	status.RestartCount = 2
	status.LastTerminationState = terminatedState(1)
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, status))
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 1)}}
	_, res := c.read(t, run, contract.AttemptKey{PodUID: podUID, Container: AgentContainer, Restart: 1}, ReadOptions{Follow: true})
	opens := c.logs.opened()
	if res.End != EndComplete || len(opens) != 1 || !opens[0].Previous || opens[0].Follow {
		t.Errorf("previous: result %+v, opens %+v", res, opens)
	}
	_, res = c.read(t, run, agentKey, ReadOptions{})
	if res.End != EndExpired || res.Reason != ReasonOlderRestart || len(c.logs.opened()) != 1 {
		t.Errorf("older: result %+v", res)
	}
}

// TestReadBackOff proves that while the container backs off, the instance
// that exited is read with Previous, which names it whatever the kubelet
// restarts meanwhile, and the one before it is expired without a read.
func TestReadBackOff(t *testing.T) {
	t.Parallel()
	const restarts = 2
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, crashLooping(restarts, 1)))
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 1)}}
	_, res := c.read(t, run, contract.AttemptKey{PodUID: podUID, Container: AgentContainer, Restart: restarts}, ReadOptions{Follow: true})
	opens := c.logs.opened()
	if res.End != EndComplete || res.Lines != 1 || len(opens) != 1 || !opens[0].Previous || opens[0].Follow {
		t.Errorf("exited: result %+v, opens %+v", res, opens)
	}
	_, res = c.read(t, run, contract.AttemptKey{PodUID: podUID, Container: AgentContainer, Restart: restarts - 1}, ReadOptions{})
	if res.End != EndExpired || res.Reason != ReasonOlderRestart || len(c.logs.opened()) != 1 {
		t.Errorf("before: result %+v", res)
	}
}

func TestReadNeverStarted(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodFailed, waiting("PodInitializing")))
	_, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
	if res.End != EndExpired || res.Reason != ReasonNeverStarted {
		t.Errorf("result = %+v", res)
	}
}

func TestReadUnavailable(t *testing.T) {
	t.Parallel()
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods/log"}, podName, errors.New("no"))
	cases := map[string]struct {
		err    error
		opens  int
		reason string
	}{
		"forbidden":    {forbidden, 1, ReasonForbidden},
		"server error": {apierrors.NewInternalError(errors.New("kubelet")), maxFailures, ReasonSourceUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
			c.logs.responses = []logResponse{{open: true, err: tc.err}}
			_, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
			if res.End != EndUnavailable || res.Reason != tc.reason || res.Coverage().State != contract.CoverageUnavailable || len(c.logs.opened()) != tc.opens {
				t.Errorf("result = %+v, opens %d", res, len(c.logs.opened()))
			}
		})
	}
}

// TestReadPodReplacedAtOpen proves a pod whose name holds another pod when
// its log opened is checked again, and the stream it opened is not read.
func TestReadPodReplacedAtOpen(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t)
	pod := newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0))
	impostor := newPod(podName, retryPodUID, jobUID, 0, corev1.PodRunning, running())
	c.podStates([]*corev1.Pod{pod}, []*corev1.Pod{impostor}, []*corev1.Pod{impostor})
	c.logs.responses = []logResponse{{text: logLine(baseTime, "the other pod's line")}}
	c.logs.onOpen = func(int) {
		// Between the check and the open, the name moved to another pod.
		if _, err := c.kube.CoreV1().Pods(testNamespace).List(context.Background(), metav1.ListOptions{}); err != nil {
			t.Error(err)
		}
	}
	got, res := c.read(t, run, agentKey, ReadOptions{})
	if len(got.lines) != 0 || res.End != EndExpired || res.Reason != ReasonPodDeleted {
		t.Errorf("lines %q, result %+v", got.texts(), res)
	}
}

func TestReadOversizedLine(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0)))
	long := strings.Repeat("x", contract.MaxLineBytes+1)
	c.logs.responses = []logResponse{{text: logLine(baseTime, long) + logLine(baseTime, "short")}}
	got, res := c.read(t, run, agentKey, ReadOptions{})
	if res.TruncatedLines != 1 || res.Coverage().TruncatedLines != 1 || len(got.lines) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if l := got.lines[0]; len(l.Text) != contract.MaxLineBytes || l.Bytes != int64(len(long)) {
		t.Errorf("held %d of %d bytes", len(l.Text), l.Bytes)
	}
}

func TestReadKnownOrigin(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0)))
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 2)}}
	_, first := c.read(t, run, agentKey, ReadOptions{})
	same, _ := c.read(t, run, agentKey, ReadOptions{KnownOrigin: first.Origin})
	if len(same.gaps) != 0 {
		t.Errorf("same origin gaps = %+v", same.gaps)
	}
	c.logs.responses = []logResponse{{text: lines(baseTime, 1, 1)}}
	rotated, res := c.read(t, run, agentKey, ReadOptions{KnownOrigin: first.Origin})
	want := []string{"gap source_truncated before 1", "line 1"}
	if !slices.Equal(rotated.order, want) || res.Origin == first.Origin || res.Gaps != 1 {
		t.Errorf("order = %q, result %+v", rotated.order, res)
	}
}

func TestReadCanceled(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
	ctx, cancel := context.WithCancel(context.Background())
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 1), err: context.Canceled}}
	c.logs.onOpen = func(int) { cancel() }
	res, err := c.src.ReadLog(ctx, run, agentKey, ReadOptions{Follow: true}, &sink{})
	if !errors.Is(err, context.Canceled) || res.End != EndCanceled || res.Coverage().Reason != ReasonCanceled {
		t.Errorf("ReadLog = %+v, %v", res, err)
	}
}

// TestReadSinkError proves the sink's error stops the read at the line
// it refused.
func TestReadSinkError(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0)))
	c.logs.responses = []logResponse{{text: lines(baseTime, 0, 3)}}
	full := errors.New("store full")
	res, err := c.src.ReadLog(context.Background(), run, agentKey, ReadOptions{}, failingSink{full})
	if !errors.Is(err, full) || res.Lines != 2 || res.End != "" {
		t.Errorf("ReadLog = %+v, %v", res, err)
	}
}

// TestReadInputLimit proves a log past contract.MaxAttemptLogBytes stops
// with an input_limit gap after the last line that fits.
func TestReadInputLimit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads more than contract.MaxAttemptLogBytes")
	}
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodSucceeded, terminated(0)))
	line := logLine(baseTime, strings.Repeat("x", contract.MaxLineBytes-len(logLine(baseTime, ""))))
	c.logs.responses = []logResponse{{text: strings.Repeat(line, contract.MaxAttemptLogBytes/len(line)+1)}}
	got, res := c.read(t, run, agentKey, ReadOptions{})
	fit := int64(contract.MaxAttemptLogBytes / len(line))
	if res.End != EndLimit || res.Lines != fit || res.Coverage().State != contract.CoverageEnded || len(got.gaps) != 1 {
		t.Fatalf("result = %+v, gaps %+v", res, got.gaps)
	}
	if g := got.gaps[0]; g.gap.Reason != contract.GapInputLimit || g.next != fit+1 {
		t.Errorf("gap = %+v", g)
	}
}

// TestReadNoResume proves a follow that broke after a line without a
// timestamp says lines may be missing rather than resume blindly.
func TestReadNoResume(t *testing.T) {
	t.Parallel()
	c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
	c.logs.responses = []logResponse{{text: "no prefix\n", err: errors.New("connection reset")}}
	got, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
	want := []string{"line 1", "gap read_interrupted before 2"}
	if !slices.Equal(got.order, want) || res.End != EndUnavailable || res.Reason != ReasonNoResume {
		t.Errorf("order = %q, result %+v", got.order, res)
	}
}

// TestReadCheckFails proves a check of the chain that fails ends the read
// as unavailable: at once when forbidden, after maxFailures otherwise.
func TestReadCheckFails(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err    error
		lists  int
		reason string
	}{
		"forbidden":    {apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("no")), 1, ReasonForbidden},
		"server error": {apierrors.NewServiceUnavailable("overloaded"), maxFailures, ReasonSourceUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, run := readCluster(t, newPod(podName, podUID, jobUID, 0, corev1.PodRunning, running()))
			lists := 0
			c.kube.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
				lists++
				return true, nil, tc.err
			})
			_, res := c.read(t, run, agentKey, ReadOptions{Follow: true})
			if res.End != EndUnavailable || res.Reason != tc.reason || lists != tc.lists || len(c.logs.opened()) != 0 {
				t.Errorf("result = %+v after %d lists", res, lists)
			}
		})
	}
}

func TestWaitCanceled(t *testing.T) {
	t.Parallel()
	c, _ := readCluster(t)
	c.src.retryDelay = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.src.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("wait = %v", err)
	}
	c.src.retryDelay = time.Nanosecond
	if err := c.src.wait(context.Background()); err != nil {
		t.Errorf("wait = %v", err)
	}
}
