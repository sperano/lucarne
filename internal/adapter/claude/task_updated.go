package claude

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Task statuses of a task_updated patch that the progress phases do not
// name: the task is not finished.
const (
	taskStatusPending = "pending"
	taskStatusRunning = "running"
	taskStatusPaused  = "paused"
)

type taskUpdatedLine struct {
	TaskID string     `json:"task_id"`
	Patch  *taskPatch `json:"patch"`
}

// taskPatch is what changed in the task. end_time and total_paused_ms stay
// in the source line: the log gives no start time to measure them from.
type taskPatch struct {
	Status         string `json:"status"`
	Description    string `json:"description"`
	Error          string `json:"error"`
	IsBackgrounded *bool  `json:"is_backgrounded"`
}

// renderTaskUpdated adds a task_updated line: progress of the call the task
// runs for when its task_started line was read and the patch has a status,
// a runner notice otherwise (a task from before the log read began, or a
// patch that changes only the description).
func (p *Parser) renderTaskUpdated(text []byte, out *adapter.Emitter) bool {
	var line taskUpdatedLine
	if json.Unmarshal(text, &line) != nil || line.Patch == nil {
		return false
	}
	patch := line.Patch
	callID, known := p.tasks.lookup(line.TaskID)
	if !known || patch.Status == "" {
		out.Emit(noticeEvent(subtypeTaskUpdated, notice{
			detail:  paragraphs(facts(patch.Status, patch.Description), patch.Error),
			isError: taskUpdatedPhase(patch.Status) == contract.ToolProgressFailed,
		}))
		return true
	}
	phase := taskUpdatedPhase(patch.Status)
	if finished(phase) {
		p.tasks.close(line.TaskID)
	}
	detail := patch.Error
	if detail == "" {
		detail = patch.Description
	}
	out.Emit(toolProgress(callID, phase, nil, detail, "", patch.IsBackgrounded))
	return true
}

// taskUpdatedPhase is the phase of a task_updated status: a task pending,
// running or paused is still running.
func taskUpdatedPhase(status string) contract.ToolProgressPhase {
	switch status {
	case taskStatusPending, taskStatusRunning, taskStatusPaused:
		return contract.ToolProgressRunning
	}
	return progressPhase(status)
}

// finished reports whether phase ends the task.
func finished(phase contract.ToolProgressPhase) bool {
	switch phase {
	case contract.ToolProgressCompleted, contract.ToolProgressFailed, contract.ToolProgressKilled:
		return true
	}
	return false
}
