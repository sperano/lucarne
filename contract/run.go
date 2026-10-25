package contract

import "encoding/json"

// Run is a Kelos Task as Lucarne last read it. Its times are the Task's
// Kubernetes lifecycle times, absent when the Task status had none.
type Run struct {
	Key       RunKey     `json:"key"`
	TaskName  string     `json:"taskName"`
	Runner    RunnerKind `json:"runner,omitempty"`
	Phase     RunPhase   `json:"phase"`
	Deleted   bool       `json:"deleted,omitempty"`
	Created   *Timestamp `json:"created,omitempty"`
	Started   *Timestamp `json:"started,omitempty"`
	Completed *Timestamp `json:"completed,omitempty"`
}

// RunPhase is the Task's phase.
type RunPhase string

// Run phases. Unknown is used when the Task has no phase Lucarne knows.
const (
	PhasePending   RunPhase = "pending"
	PhaseWaiting   RunPhase = "waiting"
	PhaseRunning   RunPhase = "running"
	PhaseSucceeded RunPhase = "succeeded"
	PhaseFailed    RunPhase = "failed"
	PhaseUnknown   RunPhase = "unknown"
)

// RunnerKind is the agent CLI whose output an attempt holds.
type RunnerKind string

// Runner kinds, named after the Kelos agent types.
const (
	RunnerClaudeCode RunnerKind = "claude-code"
	RunnerCodex      RunnerKind = "codex"
	RunnerOpenCode   RunnerKind = "opencode"
	RunnerUnknown    RunnerKind = "unknown"
)

// Runner is an attempt's runner. Version is set only when the source
// stated it.
type Runner struct {
	Kind    RunnerKind `json:"kind"`
	Version string     `json:"version,omitempty"`
}

// Attempt is one container instance of a run and how much of its log
// Lucarne holds.
type Attempt struct {
	ID        string         `json:"id"`
	Key       AttemptKey     `json:"key"`
	PodName   string         `json:"podName"`
	Runner    Runner         `json:"runner"`
	Container ContainerState `json:"container"`
	Coverage  Coverage       `json:"coverage"`
}

// ContainerState is the container status Kubernetes reported.
type ContainerState struct {
	State      ContainerStateKind `json:"state"`
	ExitCode   *int               `json:"exitCode,omitempty"`
	Reason     string             `json:"reason,omitempty"`
	StartedAt  *Timestamp         `json:"startedAt,omitempty"`
	FinishedAt *Timestamp         `json:"finishedAt,omitempty"`
}

// ContainerStateKind is the Kubernetes container state.
type ContainerStateKind string

// Container states. Unknown is a container whose status Kubernetes did not
// report.
const (
	ContainerWaiting    ContainerStateKind = "waiting"
	ContainerRunning    ContainerStateKind = "running"
	ContainerTerminated ContainerStateKind = "terminated"
	ContainerUnknown    ContainerStateKind = "unknown"
)

// CoverageState says whether an attempt's log is, was or can be read.
type CoverageState string

// Coverage states; see contract/README.md for the transitions.
const (
	CoveragePending     CoverageState = "pending"
	CoverageFollowing   CoverageState = "following"
	CoverageEnded       CoverageState = "ended"
	CoverageUnavailable CoverageState = "unavailable"
	CoverageExpired     CoverageState = "expired"
)

// Coverage is how much of an attempt's log Lucarne read. Lines counts the
// lines read, Gaps the gap events emitted, TruncatedLines the lines cut to
// MaxLineBytes.
type Coverage struct {
	State          CoverageState `json:"state"`
	Lines          int64         `json:"lines"`
	Gaps           int64         `json:"gaps"`
	TruncatedLines int64         `json:"truncatedLines"`
	Reason         string        `json:"reason,omitempty"`
}

// Stream is the position a snapshot reflects. Updates after LastSeq in the
// same Epoch follow on the SSE stream.
type Stream struct {
	Epoch         string `json:"epoch"`
	LastSeq       int64  `json:"lastSeq"`
	DroppedEvents int64  `json:"droppedEvents"`
}

// Snapshot is a run's state and retained events, from
// GET /api/v1/runs/{cluster}/{namespace}/{taskUID}.
type Snapshot struct {
	Schema   string    `json:"schema"`
	Run      Run       `json:"run"`
	Attempts []Attempt `json:"attempts"`
	Events   []Event   `json:"events"`
	Stream   Stream    `json:"stream"`
	Limits   Limits    `json:"limits"`
}

// MarshalJSON writes nil Attempts and Events as empty arrays, which the
// schema requires, so a snapshot built without them still validates.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type plain Snapshot
	if s.Attempts == nil {
		s.Attempts = []Attempt{}
	}
	if s.Events == nil {
		s.Events = []Event{}
	}
	return json.Marshal(plain(s))
}
