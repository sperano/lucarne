package kube

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// AgentContainer is the container Kelos runs the agent in, whatever
	// its type (Kelos 0.60.0 AgentContainerName).
	AgentContainer = "kelos-agent"
	// taskLabel is the label Kelos puts on a Task's Job and pods: the
	// Task's name, which a later Task can reuse.
	taskLabel = "kelos.dev/task"
	taskKind  = "Task"
	jobKind   = "Job"
)

// controlledBy reports whether obj's controller is the object of kind in
// group with uid. A name is never enough: a reused name is another object.
func controlledBy(obj metav1.Object, group, kind, uid string) bool {
	ref := metav1.GetControllerOfNoCopy(obj)
	if ref == nil || ref.Kind != kind || string(ref.UID) != uid {
		return false
	}
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	return err == nil && gv.Group == group
}

// job gets run's Job, nil when there is none controlled by run's Task: not
// created yet, deleted, or one of a Task that reused the name.
func (s *Source) job(ctx context.Context, run Run) (*batchv1.Job, error) {
	job, err := s.clients.Kube.BatchV1().Jobs(run.Key.Namespace).Get(ctx, run.jobName, metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, classify(ctx, err, "get job")
	}
	if !controlledBy(job, TaskResource.Group, taskKind, run.Key.TaskUID) {
		return nil, nil
	}
	return job, nil
}

// pods lists the pods controlled by job, oldest first.
func (s *Source) pods(ctx context.Context, run Run, job *batchv1.Job) ([]corev1.Pod, error) {
	selector := labels.Set{taskLabel: run.TaskName}.AsSelector().String()
	list, err := s.clients.Kube.CoreV1().Pods(run.Key.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, classify(ctx, err, "list pods")
	}
	var owned []corev1.Pod
	for _, pod := range list.Items {
		if controlledBy(&pod, batchv1.GroupName, jobKind, string(job.UID)) {
			owned = append(owned, pod)
		}
	}
	sortPods(owned)
	return owned, nil
}

// ownedPods is run's pods, none when its Task controls no Job.
func (s *Source) ownedPods(ctx context.Context, run Run) ([]corev1.Pod, error) {
	job, err := s.job(ctx, run)
	if err != nil || job == nil {
		return nil, err
	}
	return s.pods(ctx, run, job)
}

// ownedPod is run's pod with uid, checked through the whole chain: the Task
// by name and UID, its Job, the pod. ErrRunDeleted when the Task is gone,
// errPodGone when the pod is.
func (s *Source) ownedPod(ctx context.Context, run Run, uid string) (*corev1.Pod, error) {
	if _, err := s.taskByName(ctx, run.Key.Namespace, run.TaskName, run.Key.TaskUID); err != nil {
		return nil, err
	}
	pods, err := s.ownedPods(ctx, run)
	if err != nil {
		return nil, err
	}
	for i := range pods {
		if string(pods[i].UID) == uid {
			return &pods[i], nil
		}
	}
	return nil, fmt.Errorf("%w: pod %s", errPodGone, uid)
}

// samePod gets the pod called name again, nil when it is gone or the name
// now holds another pod.
func (s *Source) samePod(ctx context.Context, namespace, name, uid string) (*corev1.Pod, error) {
	pod, err := s.clients.Kube.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, classify(ctx, err, "get pod")
	}
	if string(pod.UID) != uid {
		return nil, nil
	}
	return pod, nil
}
