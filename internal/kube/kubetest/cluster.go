// Package kubetest is a fake Kubernetes cluster holding Kelos runs, for
// the tests that read them through the real internal/kube source: Tasks,
// their Jobs and pods in client-go's fakes, owned by UID as Kelos 0.60.0
// owns them, and container logs served as the kubelet serves them. It
// never reaches a real cluster.
package kubetest

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// Kelos 0.60.0's names for what a Task owns.
const (
	taskAPIVersion = "kelos.dev/v1alpha2"
	taskKind       = "Task"
	taskListKind   = "TaskList"
	taskLabel      = "kelos.dev/task"
	jobKind        = "Job"
)

// Task phases of Kelos 0.60.0.
const (
	PhasePending   = "Pending"
	PhaseRunning   = "Running"
	PhaseSucceeded = "Succeeded"
	PhaseFailed    = "Failed"
)

// RetryDelay is how long the source of a test cluster waits before it
// follows a log again: short, so that tests do not wait on it.
const RetryDelay = 10 * time.Millisecond

// The resources of what a Task owns. Changes go through the fakes'
// trackers, so that the actions recorded are Lucarne's only.
var (
	jobsResource = batchv1.SchemeGroupVersion.WithResource("jobs")
	podsResource = corev1.SchemeGroupVersion.WithResource("pods")
)

// readVerbs are the only verbs Lucarne may use on the API.
var readVerbs = map[string]bool{"get": true, "list": true}

// Cluster is a fake cluster named Name. It is safe for concurrent use.
type Cluster struct {
	Name string
	dyn  *dynamicfake.FakeDynamicClient
	kube *kubefake.Clientset
	logs *Logs
	// created is when the next Task is created, a second after the last.
	mu      sync.Mutex
	created time.Time
	pods    []*Pod
}

// New returns an empty cluster named name.
func New(name string) *Cluster {
	listKinds := map[schema.GroupVersionResource]string{kube.TaskResource: taskListKind}
	return &Cluster{
		Name:    name,
		dyn:     dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds),
		kube:    kubefake.NewClientset(),
		logs:    newLogs(),
		created: Epoch,
	}
}

// Clients are the clients a kube.Source reads the cluster with.
func (c *Cluster) Clients() kube.Clients {
	return kube.Clients{Dynamic: c.dyn, Kube: c.kube, Logs: c.logs}
}

// Source is a source reading the cluster's namespaces, retrying after
// RetryDelay.
func (c *Cluster) Source(namespaces ...string) (*kube.Source, error) {
	return kube.New(kube.Config{Cluster: c.Name, Namespaces: namespaces, RetryDelay: RetryDelay}, c.Clients())
}

// Writes are the calls made to the API that did not only read: none, for
// Lucarne.
func (c *Cluster) Writes() []string {
	var writes []string
	for _, a := range append(c.dyn.Actions(), c.kube.Actions()...) {
		if !readVerbs[a.GetVerb()] {
			writes = append(writes, a.GetVerb()+" "+a.GetResource().Resource)
		}
	}
	return writes
}

// Namespaces are the namespaces the API was called in, log reads included.
func (c *Cluster) Namespaces() map[string]bool {
	seen := map[string]bool{}
	for _, a := range append(c.dyn.Actions(), c.kube.Actions()...) {
		seen[a.GetNamespace()] = true
	}
	for _, o := range c.logs.Opens() {
		seen[o.Namespace] = true
	}
	return seen
}

// LogOpens are the logs opened so far, in order.
func (c *Cluster) LogOpens() []LogOpen { return c.logs.Opens() }

// Fail makes every call with verb on resource ("list", "tasks") fail with
// err until the returned function is called. It is safe to call while the
// cluster is read.
func (c *Cluster) Fail(verb, resource string, err error) (restore func()) {
	var mu sync.Mutex
	failing := true
	reactor := func(k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		return failing, nil, err
	}
	// The fakes hold their lock while they call reactors, not while one is
	// added: take it, so that a reader active meanwhile is no data race.
	for _, fake := range []*k8stesting.Fake{&c.dyn.Fake, &c.kube.Fake} {
		fake.Lock()
		fake.PrependReactor(verb, resource, reactor)
		fake.Unlock()
	}
	return func() {
		mu.Lock()
		defer mu.Unlock()
		failing = false
	}
}

// TaskSpec is a Task to create.
type TaskSpec struct {
	Namespace string
	Name      string
	UID       string
	// Agent is the Kelos agent type: claude-code, codex, opencode, or
	// one Lucarne has no adapter for.
	Agent string
	// Phase is the Kelos phase, PhaseRunning when empty.
	Phase string
}

// Run is a Task of the cluster, with its Job.
type Run struct {
	c      *Cluster
	spec   TaskSpec
	jobUID string
}

// AddTask creates a Task and the Job it controls.
func (c *Cluster) AddTask(spec TaskSpec) (*Run, error) {
	if spec.Phase == "" {
		spec.Phase = PhaseRunning
	}
	c.mu.Lock()
	created := c.created
	c.created = c.created.Add(time.Second)
	c.mu.Unlock()
	r := &Run{c: c, spec: spec, jobUID: derivedUID(spec.UID, "job")}
	if err := c.dyn.Tracker().Create(kube.TaskResource, newTask(spec, created), spec.Namespace); err != nil {
		return nil, fmt.Errorf("kubetest: create task: %w", err)
	}
	if err := c.kube.Tracker().Create(jobsResource, r.newJob(), spec.Namespace); err != nil {
		return nil, fmt.Errorf("kubetest: create job: %w", err)
	}
	return r, nil
}

// Key is the run's key.
func (r *Run) Key() contract.RunKey {
	return contract.RunKey{Cluster: r.c.Name, Namespace: r.spec.Namespace, TaskUID: r.spec.UID}
}

// SetPhase changes the Task's phase.
func (r *Run) SetPhase(phase string) error {
	tracker := r.c.dyn.Tracker()
	obj, err := tracker.Get(kube.TaskResource, r.spec.Namespace, r.spec.Name)
	if err != nil {
		return fmt.Errorf("kubetest: get task: %w", err)
	}
	task, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("kubetest: task is a %T", obj)
	}
	if err := unstructured.SetNestedField(task.Object, phase, "status", "phase"); err != nil {
		return fmt.Errorf("kubetest: set phase: %w", err)
	}
	if err := tracker.Update(kube.TaskResource, task, r.spec.Namespace); err != nil {
		return fmt.Errorf("kubetest: update task: %w", err)
	}
	return nil
}

// Delete deletes the Task, its Job and its pods, as Kelos does when the
// finished Task's TTL runs out.
func (r *Run) Delete() error {
	ns := r.spec.Namespace
	if err := r.c.dyn.Tracker().Delete(kube.TaskResource, ns, r.spec.Name); err != nil {
		return fmt.Errorf("kubetest: delete task: %w", err)
	}
	if err := r.c.kube.Tracker().Delete(jobsResource, ns, r.spec.Name); err != nil {
		return fmt.Errorf("kubetest: delete job: %w", err)
	}
	for _, pod := range r.c.podsOf(ns, r.spec.Name) {
		if err := pod.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func newTask(spec TaskSpec, created time.Time) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": taskAPIVersion,
		"kind":       taskKind,
		"metadata": map[string]any{
			"name":              spec.Name,
			"namespace":         spec.Namespace,
			"uid":               spec.UID,
			"creationTimestamp": created.Format(time.RFC3339),
		},
		"spec":   map[string]any{"worker": map[string]any{"type": spec.Agent}, "prompt": "fabricated prompt"},
		"status": map[string]any{"phase": spec.Phase, "jobName": spec.Name},
	}}
}

func (r *Run) newJob() *batchv1.Job {
	controller := true
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: r.spec.Name, Namespace: r.spec.Namespace, UID: types.UID(r.jobUID),
		Labels: map[string]string{taskLabel: r.spec.Name},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: taskAPIVersion, Kind: taskKind, Name: r.spec.Name, UID: types.UID(r.spec.UID), Controller: &controller,
		}},
	}}
}

// derivedUID is a UID made from uid, a UUID, for an object of kind: its
// last group becomes a hash of kind, so each kind gets its own UID.
func derivedUID(uid, kind string) string {
	const (
		prefix   = len("00000000-0000-0000-0000-")
		lastMask = 1<<48 - 1 // the 12 hex digits of the last group
	)
	h := fnv.New64a()
	_, _ = h.Write([]byte(kind)) // a hash.Hash never fails to write
	return fmt.Sprintf("%s%012x", uid[:prefix], h.Sum64()&lastMask)
}
