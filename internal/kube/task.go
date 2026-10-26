package kube

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sperano/lucarne/contract"
)

// task holds the fields of a Kelos Task the source uses. Everything else
// in the object, the prompt included, is dropped when it is decoded.
type task struct {
	// UID, Name and Created come from the object's metadata.
	UID     string      `json:"-"`
	Name    string      `json:"-"`
	Created metav1.Time `json:"-"`
	Spec    taskSpec    `json:"spec"`
	Status  taskStatus  `json:"status"`
}

type taskSpec struct {
	// Type is the deprecated agent type; Worker.Type takes precedence.
	Type   string `json:"type"`
	Worker struct {
		Type string `json:"type"`
	} `json:"worker"`
}

type taskStatus struct {
	Phase          string       `json:"phase"`
	JobName        string       `json:"jobName"`
	StartTime      *metav1.Time `json:"startTime"`
	CompletionTime *metav1.Time `json:"completionTime"`
}

// Task phases of Kelos 0.60.0.
const (
	taskPending   = "Pending"
	taskWaiting   = "Waiting"
	taskRunning   = "Running"
	taskSucceeded = "Succeeded"
	taskFailed    = "Failed"
)

var taskPhases = map[string]contract.RunPhase{
	taskPending:   contract.PhasePending,
	taskWaiting:   contract.PhaseWaiting,
	taskRunning:   contract.PhaseRunning,
	taskSucceeded: contract.PhaseSucceeded,
	taskFailed:    contract.PhaseFailed,
}

// runners maps a Kelos agent type to the runner whose output its log holds.
// Kelos also has gemini and cursor, for which Lucarne has no adapter.
var runners = map[string]contract.RunnerKind{
	string(contract.RunnerClaudeCode): contract.RunnerClaudeCode,
	string(contract.RunnerCodex):      contract.RunnerCodex,
	string(contract.RunnerOpenCode):   contract.RunnerOpenCode,
}

// decodeTask reads the fields the source uses from a Task object.
func decodeTask(obj *unstructured.Unstructured) (task, error) {
	var t task
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &t); err != nil {
		return task{}, fmt.Errorf("decode task %s: %w", obj.GetName(), err)
	}
	t.UID, t.Name = string(obj.GetUID()), obj.GetName()
	t.Created = obj.GetCreationTimestamp()
	return t, nil
}

// runner is the Task's runner, unknown for an agent type without adapter
// and empty when the Task names none.
func (t task) runner() contract.RunnerKind {
	agent := t.Spec.Worker.Type
	if agent == "" {
		agent = t.Spec.Type
	}
	if agent == "" {
		return ""
	}
	if kind, ok := runners[agent]; ok {
		return kind
	}
	return contract.RunnerUnknown
}

func (t task) phase() contract.RunPhase {
	if phase, ok := taskPhases[t.Status.Phase]; ok {
		return phase
	}
	return contract.PhaseUnknown
}

// jobName is the Task's Job: the status names it once Kelos created it, and
// Kelos names it after the Task. Its owner is checked by UID either way.
func (t task) jobName() string {
	if t.Status.JobName != "" {
		return t.Status.JobName
	}
	return t.Name
}

func (t task) contractRun(key contract.RunKey) contract.Run {
	return contract.Run{
		Key:       key,
		TaskName:  t.Name,
		Runner:    t.runner(),
		Phase:     t.phase(),
		Created:   timestamp(&t.Created),
		Started:   timestamp(t.Status.StartTime),
		Completed: timestamp(t.Status.CompletionTime),
	}
}

// timestamp is t in the contract, nil when Kubernetes gave no time.
func timestamp(t *metav1.Time) *contract.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return &contract.Timestamp{Time: t.UTC()}
}
