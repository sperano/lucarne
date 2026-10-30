package claude

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

const millisecondsPerSecond int64 = 1000

type taskProgressLine struct {
	ToolUseID      string `json:"tool_use_id"`
	Status         string `json:"status"`
	Description    string `json:"description"`
	Summary        string `json:"summary"`
	TaskType       string `json:"task_type"`
	IsBackgrounded *bool  `json:"is_backgrounded"`
	// SubagentType and Usage are on task_progress lines only.
	SubagentType string             `json:"subagent_type"`
	Usage        *taskProgressUsage `json:"usage"`
}

// taskProgressUsage is a task_progress line's running total; only its
// duration is rendered, the token and tool counts stay in the source line.
type taskProgressUsage struct {
	DurationMs *int64 `json:"duration_ms"`
}

type toolProgressLine struct {
	ParentToolUseID   string `json:"parent_tool_use_id"`
	ElapsedTimeSecond *int64 `json:"elapsed_time_seconds"`
}

func renderTaskProgress(subtype string, text []byte, out *adapter.Emitter) bool {
	var line taskProgressLine
	if json.Unmarshal(text, &line) != nil || line.ToolUseID == "" {
		return false
	}
	switch subtype {
	case subtypeTaskProgress:
		out.Emit(toolProgress(
			line.ToolUseID, contract.ToolProgressRunning, line.durationMs(), line.Description, line.SubagentType, nil,
		))
	case subtypeTaskNotification:
		out.Emit(toolProgress(
			line.ToolUseID, progressPhase(line.Status), nil, line.Summary, line.TaskType, line.IsBackgrounded,
		))
	default:
		out.Emit(toolProgress(
			line.ToolUseID, contract.ToolProgressStarted, nil, line.Description, line.TaskType, line.IsBackgrounded,
		))
	}
	return true
}

// durationMs is the task's elapsed time a task_progress line reports, nil
// when it reports none or an invalid one.
func (l taskProgressLine) durationMs() *int64 {
	if l.Usage == nil {
		return nil
	}
	return adapter.Count(l.Usage.DurationMs)
}

func renderToolProgress(text []byte, out *adapter.Emitter) bool {
	var line toolProgressLine
	if json.Unmarshal(text, &line) != nil || line.ParentToolUseID == "" {
		return false
	}
	out.Emit(toolProgress(
		line.ParentToolUseID,
		contract.ToolProgressRunning,
		elapsedMilliseconds(line.ElapsedTimeSecond),
		"",
		"",
		nil,
	))
	return true
}

func toolProgress(
	callID string,
	phase contract.ToolProgressPhase,
	elapsedMs *int64,
	detail string,
	subtype string,
	background *bool,
) contract.Event {
	var cut adapter.Truncation
	progress := &contract.ToolProgress{
		CallID:     cut.Text("callId", callID, contract.MaxLabelBytes),
		Phase:      phase,
		ElapsedMs:  elapsedMs,
		Detail:     cut.Text("detail", detail, contract.MaxTextBytes),
		Subtype:    cut.Text("subtype", subtype, contract.MaxLabelBytes),
		Background: background,
	}
	return contract.Event{Kind: contract.KindToolProgress, ToolProgress: progress, Truncated: cut.Fields()}
}

func progressPhase(status string) contract.ToolProgressPhase {
	switch status {
	case "completed":
		return contract.ToolProgressCompleted
	case "failed":
		return contract.ToolProgressFailed
	case "killed":
		return contract.ToolProgressKilled
	default:
		return contract.ToolProgressUnknown
	}
}

func elapsedMilliseconds(seconds *int64) *int64 {
	if seconds == nil || *seconds < 0 || *seconds > adapter.MaxCount/millisecondsPerSecond {
		return nil
	}
	milliseconds := *seconds * millisecondsPerSecond
	return &milliseconds
}
