package contract

import "encoding/json"

// EventKind discriminates an Event's payload.
type EventKind string

// Event kinds. Exactly the payload field named after the kind is set.
const (
	KindMessage    EventKind = "message"
	KindToolCall   EventKind = "tool_call"
	KindToolResult EventKind = "tool_result"
	KindEdit       EventKind = "edit"
	KindUsage      EventKind = "usage"
	KindError      EventKind = "error"
	KindLifecycle  EventKind = "lifecycle"
	KindRaw        EventKind = "raw"
	KindGap        EventKind = "gap"
)

// EventKinds lists every kind, in the schema's order.
var EventKinds = []EventKind{
	KindMessage, KindToolCall, KindToolResult, KindEdit, KindUsage,
	KindError, KindLifecycle, KindRaw, KindGap,
}

// Event is one normalized transcript event. Seq orders it in its stream;
// ID names the source position it came from.
type Event struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	Attempt   string    `json:"attempt"`
	Kind      EventKind `json:"kind"`
	Source    Source    `json:"source"`
	Truncated []string  `json:"truncated,omitempty"`

	Message    *Message    `json:"message,omitempty"`
	ToolCall   *ToolCall   `json:"toolCall,omitempty"`
	ToolResult *ToolResult `json:"toolResult,omitempty"`
	Edit       *Edit       `json:"edit,omitempty"`
	Usage      *Usage      `json:"usage,omitempty"`
	Error      *ErrorInfo  `json:"error,omitempty"`
	Lifecycle  *Lifecycle  `json:"lifecycle,omitempty"`
	Raw        *Raw        `json:"raw,omitempty"`
	Gap        *Gap        `json:"gap,omitempty"`
}

// Source is where an event came from in its attempt's log. LogTime is the
// container runtime's receipt time of the line; RunnerTime is a time the
// runner wrote into the line. Either is absent when the source had none.
type Source struct {
	Line       int64      `json:"line"`
	Part       int        `json:"part"`
	Type       string     `json:"type,omitempty"`
	LogTime    *Timestamp `json:"logTime,omitempty"`
	RunnerTime *Timestamp `json:"runnerTime,omitempty"`
}

// Message is text the runner attributed to the user or the assistant.
type Message struct {
	Role      string `json:"role"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	MessageID string `json:"messageId,omitempty"`
}

// ToolCall is a tool invocation. Input holds the source's input when it
// fits MaxToolInputBytes; otherwise InputText holds its cut serialization.
type ToolCall struct {
	CallID       string          `json:"callId"`
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input,omitempty"`
	InputText    string          `json:"inputText,omitempty"`
	Title        string          `json:"title,omitempty"`
	ParentCallID string          `json:"parentCallId,omitempty"`
}

// ToolResult is the outcome of the tool call with the same CallID.
type ToolResult struct {
	CallID   string `json:"callId"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

// Edit is a file change, with whichever of a diff or before/after text the
// source carried.
type Edit struct {
	CallID string `json:"callId,omitempty"`
	Path   string `json:"path"`
	Change string `json:"change"`
	Diff   string `json:"diff,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// Usage holds only the counters the source reported, over Scope.
type Usage struct {
	Scope            string `json:"scope"`
	InputTokens      *int64 `json:"inputTokens,omitempty"`
	OutputTokens     *int64 `json:"outputTokens,omitempty"`
	CacheReadTokens  *int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int64 `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  *int64 `json:"reasoningTokens,omitempty"`
	TotalTokens      *int64 `json:"totalTokens,omitempty"`
	CostUSD          string `json:"costUSD,omitempty"`
	DurationMs       *int64 `json:"durationMs,omitempty"`
	APIDurationMs    *int64 `json:"apiDurationMs,omitempty"`
	Turns            *int64 `json:"turns,omitempty"`
}

// ErrorInfo is an error the runner reported; Retry is set when the source
// said it will retry.
type ErrorInfo struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Status  *int   `json:"status,omitempty"`
	Retry   *Retry `json:"retry,omitempty"`
}

// Retry is the retry the source announced.
type Retry struct {
	Attempt int    `json:"attempt"`
	Max     *int   `json:"max,omitempty"`
	DelayMs *int64 `json:"delayMs,omitempty"`
}

// Lifecycle is a session, turn or Kelos marker the source emitted.
type Lifecycle struct {
	Phase         string   `json:"phase"`
	SessionID     string   `json:"sessionId,omitempty"`
	Model         string   `json:"model,omitempty"`
	RunnerVersion string   `json:"runnerVersion,omitempty"`
	Subtype       string   `json:"subtype,omitempty"`
	IsError       *bool    `json:"isError,omitempty"`
	StopReason    string   `json:"stopReason,omitempty"`
	ExitCode      *int     `json:"exitCode,omitempty"`
	Detail        string   `json:"detail,omitempty"`
	Outputs       []Output `json:"outputs,omitempty"`
}

// Output is one "key: value" line of the Kelos outputs block.
type Output struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Raw is a line, or the part of one, no adapter rendered.
type Raw struct {
	Reason string `json:"reason"`
	Text   string `json:"text"`
	Bytes  int64  `json:"bytes"`
}

// Gap says that lines are missing before Source.Line.
type Gap struct {
	Reason string `json:"reason"`
	Lines  *int64 `json:"lines,omitempty"`
	Detail string `json:"detail,omitempty"`
}
