package kubetest

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sperano/lucarne/internal/kube"
)

// Epoch is when the cluster's first Task was created; log lines are
// stamped from it on.
var Epoch = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

// initContainer is a container Kelos runs before the agent's, whose log
// Lucarne never reads.
const initContainer = "git-clone"

// Pod is a pod of a run's Job, its agent container running until
// Terminate.
type Pod struct {
	c         *Cluster
	namespace string
	name      string
	// task is the name of the Task the pod is labelled for.
	task string
	log  *podLog
}

// AddPod creates a pod of the run's Job, with its agent container running
// and an empty log.
func (r *Run) AddPod(name, uid string) (*Pod, error) {
	pod := newPod(r.spec.Namespace, name, uid, r.spec.Name, r.jobUID)
	return r.c.addPod(pod)
}

// AddForeignPod creates a pod in namespace that carries the label of the
// Task called taskName, as its own pods do, but that its Job does not
// control: Lucarne must never read it.
func (c *Cluster) AddForeignPod(namespace, name, uid, taskName string) (*Pod, error) {
	return c.addPod(newPod(namespace, name, uid, taskName, derivedUID(uid, "foreign")))
}

func (c *Cluster) addPod(pod *corev1.Pod) (*Pod, error) {
	if err := c.kube.Tracker().Create(podsResource, pod, pod.Namespace); err != nil {
		return nil, fmt.Errorf("kubetest: create pod: %w", err)
	}
	p := &Pod{c: c, namespace: pod.Namespace, name: pod.Name, task: pod.Labels[taskLabel], log: c.logs.add(pod.Namespace, pod.Name)}
	c.mu.Lock()
	c.pods = append(c.pods, p)
	c.mu.Unlock()
	return p, nil
}

// podsOf are the pods labelled for the Task called task in namespace.
func (c *Cluster) podsOf(namespace, task string) []*Pod {
	c.mu.Lock()
	defer c.mu.Unlock()
	var pods []*Pod
	for _, p := range c.pods {
		if p.namespace == namespace && p.task == task {
			pods = append(pods, p)
		}
	}
	return pods
}

// Write appends lines to the agent container's log, each stamped a
// LineInterval after the previous one.
func (p *Pod) Write(lines ...string) { p.log.write(lines) }

// Lines is how many lines the log holds.
func (p *Pod) Lines() int { return p.log.count() }

// Break breaks the log streams open now, as a dropped connection to the
// kubelet does: the source reads the log again.
func (p *Pod) Break() { p.log.breakStreams() }

// Rotate makes the log start after the lines it holds now, as the
// kubelet does once it rotated the log file: a later read no longer
// finds them.
func (p *Pod) Rotate() { p.log.rotate() }

// Terminate ends the agent container with exitCode, and the pod with it:
// Succeeded for 0, else Failed. The log is complete.
func (p *Pod) Terminate(exitCode int32) error {
	phase, reason := corev1.PodSucceeded, "Completed"
	if exitCode != 0 {
		phase, reason = corev1.PodFailed, "Error"
	}
	err := p.update(func(pod *corev1.Pod) {
		pod.Status.Phase = phase
		pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			ExitCode: exitCode, Reason: reason, StartedAt: pod.CreationTimestamp, FinishedAt: metav1.NewTime(p.log.lastStamp()),
		}}
	})
	if err != nil {
		return err
	}
	p.log.terminate()
	return nil
}

// Delete deletes the pod, as an eviction or a node loss does.
func (p *Pod) Delete() error {
	if err := p.c.kube.Tracker().Delete(podsResource, p.namespace, p.name); err != nil {
		return fmt.Errorf("kubetest: delete pod: %w", err)
	}
	p.log.terminate()
	return nil
}

func (p *Pod) update(change func(*corev1.Pod)) error {
	tracker := p.c.kube.Tracker()
	obj, err := tracker.Get(podsResource, p.namespace, p.name)
	if err != nil {
		return fmt.Errorf("kubetest: get pod: %w", err)
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("kubetest: pod is a %T", obj)
	}
	change(pod)
	if err := tracker.Update(podsResource, pod, p.namespace); err != nil {
		return fmt.Errorf("kubetest: update pod: %w", err)
	}
	return nil
}

// newPod is a running pod labelled for taskName and controlled by the Job
// with jobUID.
func newPod(namespace, name, uid, taskName, jobUID string) *corev1.Pod {
	controller := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: namespace, UID: types.UID(uid),
			CreationTimestamp: metav1.NewTime(Epoch),
			Labels:            map[string]string{taskLabel: taskName},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: jobKind, Name: taskName, UID: types.UID(jobUID), Controller: &controller,
			}},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: initContainer}},
			Containers:     []corev1.Container{{Name: kube.AgentContainer}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  kube.AgentContainer,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(Epoch)}},
			}},
		},
	}
}
