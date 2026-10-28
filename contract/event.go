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
	Role      MessageRole `json:"role"`
	Type      MessageType `json:"type"`
	Text      string      `json:"text"`
	MessageID string      `json:"messageId,omitempty"`
}

// MessageRole is who the runner attributed a message to.
type MessageRole string

// Message roles.
const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// MessageType says whether a message is answer text or reasoning.
type MessageType string

// Message types.
const (
	MessageText      MessageType = "text"
	MessageReasoning MessageType = "reasoning"
)

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

// ToolResult is the outcome of the tool call with the same CallID. Output
// is nil when the source reported none, and empty when the tool printed
// nothing.
type ToolResult struct {
	CallID   string           `json:"callId"`
	Status   ToolResultStatus `json:"status"`
	Output   *string          `json:"output,omitempty"`
	ExitCode *int             `json:"exitCode,omitempty"`
}

// ToolResultStatus is a tool call's outcome as the runner reported it.
type ToolResultStatus string

// Tool result statuses. Unknown is a result whose source stated none.
const (
	ResultOK       ToolResultStatus = "ok"
	ResultError    ToolResultStatus = "error"
	ResultDeclined ToolResultStatus = "declined"
	ResultUnknown  ToolResultStatus = "unknown"
)

// Edit is a file change, with whichever of a diff or before/after text the
// source carried; a nil text is one the source did not carry, an empty one
// is empty content (an emptied file).
type Edit struct {
	CallID string     `json:"callId,omitempty"`
	Path   string     `json:"path"`
	Change EditChange `json:"change"`
	Diff   *string    `json:"diff,omitempty"`
	Before *string    `json:"before,omitempty"`
	After  *string    `json:"after,omitempty"`
}

// EditChange is what an edit did to its file.
type EditChange string

// Edit changes. Unknown is a change whose source did not say which.
const (
	ChangeAdd     EditChange = "add"
	ChangeUpdate  EditChange = "update"
	ChangeDelete  EditChange = "delete"
	ChangeUnknown EditChange = "unknown"
)

// Usage holds only the counters the source reported, over Scope.
type Usage struct {
	Scope            UsageScope `json:"scope"`
	InputTokens      *int64     `json:"inputTokens,omitempty"`
	OutputTokens     *int64     `json:"outputTokens,omitempty"`
	CacheReadTokens  *int64     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int64     `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens  *int64     `json:"reasoningTokens,omitempty"`
	TotalTokens      *int64     `json:"totalTokens,omitempty"`
	CostUSD          string     `json:"costUSD,omitempty"`
	DurationMs       *int64     `json:"durationMs,omitempty"`
	APIDurationMs    *int64     `json:"apiDurationMs,omitempty"`
	Turns            *int64     `json:"turns,omitempty"`
}

// UsageScope is what a usage report covers, as the runner defines it.
type UsageScope string

// Usage scopes.
const (
	ScopeMessage UsageScope = "message"
	ScopeStep    UsageScope = "step"
	ScopeTurn    UsageScope = "turn"
	ScopeSession UsageScope = "session"
)

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

// Lifecycle is a session, turn or Kelos marker the source emitted. Detail
// is nil when the source line had none.
type Lifecycle struct {
	Phase         LifecyclePhase `json:"phase"`
	SessionID     string         `json:"sessionId,omitempty"`
	Model         string         `json:"model,omitempty"`
	RunnerVersion string         `json:"runnerVersion,omitempty"`
	Subtype       string         `json:"subtype,omitempty"`
	IsError       *bool          `json:"isError,omitempty"`
	StopReason    string         `json:"stopReason,omitempty"`
	ExitCode      *int           `json:"exitCode,omitempty"`
	Detail        *string        `json:"detail,omitempty"`
	Outputs       []Output       `json:"outputs,omitempty"`
}

// LifecyclePhase names a session, turn or Kelos marker.
type LifecyclePhase string

// Lifecycle phases; see contract/README.md for the source of each.
const (
	LifecycleSetupStarted     LifecyclePhase = "setup_started"
	LifecycleSetupCompleted   LifecyclePhase = "setup_completed"
	LifecycleSetupFailed      LifecyclePhase = "setup_failed"
	LifecycleSessionStarted   LifecyclePhase = "session_started"
	LifecycleStepStarted      LifecyclePhase = "step_started"
	LifecycleStepCompleted    LifecyclePhase = "step_completed"
	LifecycleTurnStarted      LifecyclePhase = "turn_started"
	LifecycleTurnCompleted    LifecyclePhase = "turn_completed"
	LifecycleTurnFailed       LifecyclePhase = "turn_failed"
	LifecycleCompacted        LifecyclePhase = "compacted"
	LifecycleSessionCompleted LifecyclePhase = "session_completed"
	LifecycleRateLimitStatus  LifecyclePhase = "rate_limit_status"
	LifecycleLimitHit         LifecyclePhase = "limit_hit"
	LifecycleKelosOutputs     LifecyclePhase = "kelos_outputs"
)

// Output is one "key: value" line of the Kelos outputs block.
type Output struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Raw is a line, or the part of one, no adapter rendered.
type Raw struct {
	Reason RawReason `json:"reason"`
	Text   string    `json:"text"`
	Bytes  int64     `json:"bytes"`
}

// RawReason says why no adapter rendered a line.
type RawReason string

// Raw reasons; see contract/README.md.
const (
	RawText          RawReason = "text"
	RawMalformedJSON RawReason = "malformed_json"
	RawUnknownType   RawReason = "unknown_type"
	RawUnsupported   RawReason = "unsupported"
	RawOversized     RawReason = "oversized"
)

// Gap says that lines are missing before Source.Line.
type Gap struct {
	Reason GapReason `json:"reason"`
	Lines  *int64    `json:"lines,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// GapReason says why lines are missing.
type GapReason string

// Gap reasons; see contract/README.md.
const (
	GapSourceTruncated GapReason = "source_truncated"
	GapReadInterrupted GapReason = "read_interrupted"
	GapInputLimit      GapReason = "input_limit"
	GapAttemptExpired  GapReason = "attempt_expired"
)
