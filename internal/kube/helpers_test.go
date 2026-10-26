package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Fabricated identities: the repository is public.
const (
	testCluster   = "example"
	testNamespace = "agents"
	otherNS       = "other"
	taskName      = "example-claude-1"
	taskUID       = "3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"
	reusedUID     = "4a9f3b5d-2c3e-4f60-9bac-1d2e3f4a5b6c"
	jobUID        = "5b0a4c6e-3d4f-4a71-8cbd-2e3f4a5b6c7d"
	podName       = "example-claude-1-abcde"
	podUID        = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f"
	retryPodName  = "example-claude-1-fghij"
	retryPodUID   = "8d0e1f2a-3b4c-4d5e-8f70-8b9c0d1e2f3a"
)

// baseTime is when the fabricated Task was created.
var baseTime = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

// readVerbs are the only verbs the source may use.
var readVerbs = map[string]bool{"get": true, "list": true}

// cluster is a fake Kubernetes API and log source.
type cluster struct {
	dyn  *dynamicfake.FakeDynamicClient
	kube *kubefake.Clientset
	logs *fakeLogs
	src  *Source
}

func newCluster(t *testing.T, tasks []*unstructured.Unstructured, objects ...runtime.Object) *cluster {
	t.Helper()
	dynObjects := make([]runtime.Object, len(tasks))
	for i, task := range tasks {
		dynObjects[i] = task
	}
	listKinds := map[schema.GroupVersionResource]string{TaskResource: "TaskList"}
	c := &cluster{
		dyn:  dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, dynObjects...),
		kube: kubefake.NewClientset(objects...),
		logs: &fakeLogs{},
	}
	src, err := New(Config{Cluster: testCluster, Namespaces: []string{testNamespace}}, Clients{Dynamic: c.dyn, Kube: c.kube, Logs: c.logs})
	if err != nil {
		t.Fatal(err)
	}
	src.retryDelay = 0
	c.src = src
	return c
}

// assertReadOnly fails unless every call the source made only read.
func (c *cluster) assertReadOnly(t *testing.T) {
	t.Helper()
	for _, a := range append(c.dyn.Actions(), c.kube.Actions()...) {
		if !readVerbs[a.GetVerb()] {
			t.Errorf("source called %s %s", a.GetVerb(), a.GetResource().Resource)
		}
	}
}

// calls is the number of API calls and log opens the source made.
func (c *cluster) calls() int {
	return len(c.dyn.Actions()) + len(c.kube.Actions()) + len(c.logs.opened())
}

// run resolves the fabricated Task.
func (c *cluster) run(t *testing.T) Run {
	t.Helper()
	run, err := c.src.Run(context.Background(), contract.RunKey{Cluster: testCluster, Namespace: testNamespace, TaskUID: taskUID})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func newTask(namespace, name, uid, phase, agent string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kelos.dev/v1alpha2",
		"kind":       "Task",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"uid":               uid,
			"creationTimestamp": baseTime.Format(time.RFC3339),
		},
		"spec":   map[string]any{"worker": map[string]any{"type": agent}, "prompt": "fabricated prompt"},
		"status": map[string]any{"phase": phase, "jobName": name},
	}}
	return obj
}

func newJob(name, uid, ownerUID string) *batchv1.Job {
	controller := true
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: testNamespace, UID: types.UID(uid),
		Labels: map[string]string{taskLabel: name},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "kelos.dev/v1alpha2", Kind: taskKind, Name: name, UID: types.UID(ownerUID), Controller: &controller,
		}},
	}}
}

// newPod is a pod of the Task's Job, created offset after baseTime, with
// its agent container in status.
func newPod(name, uid, ownerUID string, offset time.Duration, phase corev1.PodPhase, status *corev1.ContainerStatus) *corev1.Pod {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: testNamespace, UID: types.UID(uid),
			CreationTimestamp: metav1.NewTime(baseTime.Add(offset)),
			Labels:            map[string]string{taskLabel: taskName},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: jobKind, Name: taskName, UID: types.UID(ownerUID), Controller: &controller,
			}},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "git-clone"}},
			Containers:     []corev1.Container{{Name: AgentContainer}},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	if status != nil {
		status.Name = AgentContainer
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{*status}
	}
	return pod
}

func running() *corev1.ContainerStatus {
	return &corev1.ContainerStatus{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(baseTime)}}}
}

func terminated(code int32) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{State: terminatedState(code)}
}

func terminatedState(code int32) corev1.ContainerState {
	return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
		ExitCode: code, Reason: "Completed", StartedAt: metav1.NewTime(baseTime), FinishedAt: metav1.NewTime(baseTime.Add(time.Minute)),
	}}
}

// crashLooping is the status the kubelet reports while the agent container
// backs off after instance restarts exited: that instance is both the
// status's container and its last termination.
func crashLooping(restarts int32, code int32) *corev1.ContainerStatus {
	status := waiting("CrashLoopBackOff")
	status.RestartCount = restarts
	status.ContainerID = "containerd://exited"
	status.LastTerminationState = terminatedState(code)
	status.LastTerminationState.Terminated.ContainerID = status.ContainerID
	return status
}

func waiting(reason string) *corev1.ContainerStatus {
	return &corev1.ContainerStatus{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}
}

// podStates makes the fake API answer each list of pods with the next of
// states, and each get with the pod of that name in the current one; the
// last state stays.
func (c *cluster) podStates(states ...[]*corev1.Pod) {
	var mu sync.Mutex
	current := -1
	c.kube.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		current = min(current+1, len(states)-1)
		list := &corev1.PodList{}
		for _, pod := range states[current] {
			list.Items = append(list.Items, *pod)
		}
		return true, list, nil
	})
	c.kube.PrependReactor("get", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		name := a.(k8stesting.GetAction).GetName()
		for _, pod := range states[max(current, 0)] {
			if pod.Name == name {
				return true, pod, nil
			}
		}
		return true, nil, notFound("pods", name)
	})
}

// logLine is one line as the log API writes it with timestamps.
func logLine(at time.Time, text string) string {
	return at.UTC().Format(time.RFC3339Nano) + " " + text + "\n"
}

// fakeLogs serves scripted log streams: each open of a log takes the next
// response, the last one repeating.
type fakeLogs struct {
	mu        sync.Mutex
	responses []logResponse
	next      int
	opens     []corev1.PodLogOptions
	// onOpen runs after each open, before the stream is read.
	onOpen func(open int)
}

// logResponse is a log stream: text, which a read since a time filters as
// the kubelet does, then err (nil for a clean end), or err instead of a
// stream when open is set.
type logResponse struct {
	text string
	err  error
	open bool
}

func (f *fakeLogs) Stream(_ context.Context, _, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	f.opens = append(f.opens, *opts)
	open := len(f.opens)
	if len(f.responses) == 0 {
		f.mu.Unlock()
		return nil, errors.New("fakeLogs: no response scripted")
	}
	resp := f.responses[min(f.next, len(f.responses)-1)]
	f.next++
	hook := f.onOpen
	f.mu.Unlock()
	if hook != nil {
		hook(open)
	}
	if resp.open {
		return nil, resp.err
	}
	text := resp.text
	if opts.SinceTime != nil {
		text = since(text, opts.SinceTime.Time)
	}
	return io.NopCloser(io.MultiReader(strings.NewReader(text), errReader{resp.err})), nil
}

func (f *fakeLogs) opened() []corev1.PodLogOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]corev1.PodLogOptions(nil), f.opens...)
}

// since keeps the lines of text stamped at or after t.
func since(text string, t time.Time) string {
	var kept strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		stamp, _, _ := strings.Cut(line, " ")
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil && !at.Before(t) {
			kept.WriteString(line)
		}
	}
	return kept.String()
}

// errReader ends a stream with err, io.EOF when nil.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) {
	if r.err == nil {
		return 0, io.EOF
	}
	return 0, r.err
}

// sink records what a read delivers, in order.
type sink struct {
	lines   []adapter.Line
	gaps    []sinkGap
	origins map[string]bool
	order   []string
}

type sinkGap struct {
	next int64
	gap  contract.Gap
}

func (s *sink) Line(origin string, line adapter.Line) error {
	s.note(origin)
	s.lines = append(s.lines, line)
	s.order = append(s.order, fmt.Sprintf("line %d", line.Number))
	return nil
}

func (s *sink) Gap(origin string, next int64, gap contract.Gap) error {
	s.note(origin)
	s.gaps = append(s.gaps, sinkGap{next: next, gap: gap})
	s.order = append(s.order, fmt.Sprintf("gap %s before %d", gap.Reason, next))
	return nil
}

func (s *sink) note(origin string) {
	if s.origins == nil {
		s.origins = map[string]bool{}
	}
	s.origins[origin] = true
}

// texts is the text of each line delivered.
func (s *sink) texts() []string {
	texts := make([]string, len(s.lines))
	for i, l := range s.lines {
		texts[i] = string(l.Text)
	}
	return texts
}

// lines writes count numbered lines, one per millisecond from start.
func lines(start time.Time, from, count int) string {
	var b bytes.Buffer
	for i := from; i < from+count; i++ {
		b.WriteString(logLine(start.Add(time.Duration(i)*time.Millisecond), fmt.Sprintf(`{"n":%d}`, i)))
	}
	return b.String()
}

func notFound(resource, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: resource}, name)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// failingSink fails on the second line.
type failingSink struct{ err error }

func (f failingSink) Line(_ string, line adapter.Line) error {
	if line.Number > 1 {
		return f.err
	}
	return nil
}

func (f failingSink) Gap(string, int64, contract.Gap) error { return f.err }
