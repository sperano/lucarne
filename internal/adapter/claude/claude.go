package claude

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Line types of stream-json.
const (
	typeSystem    = "system"
	typeAssistant = "assistant"
	typeUser      = "user"
	typeResult    = "result"
	// typeRateLimit reports the account's rate limit status.
	typeRateLimit = "rate_limit_event"
	// typeToolProgress is a heartbeat the CLI prints while a tool call (a
	// long Bash command, say) is still running; it carries the only elapsed
	// time for that call, which tool_use and tool_result do not.
	typeToolProgress = "tool_progress"
)

// System subtypes the parser renders.
const (
	subtypeInit            = "init"
	subtypeAPIRetry        = "api_retry"
	subtypeCompactBoundary = "compact_boundary"
	subtypeThinkingTokens  = "thinking_tokens"
)

// System subtypes for the CLI's background monitoring of a tool call.
const (
	subtypeTaskStarted      = "task_started"
	subtypeTaskNotification = "task_notification"
	// subtypeTaskProgress is a subagent's running report: its description,
	// type, elapsed time and token and tool counts so far.
	subtypeTaskProgress = "task_progress"
	// subtypeTaskUpdated is a change of a task's state; it names the task,
	// not the tool call.
	subtypeTaskUpdated = "task_updated"
)

// renderedSubtypes are the system subtypes the parser renders, runner
// notices aside.
var renderedSubtypes = map[string]bool{
	subtypeInit: true, subtypeAPIRetry: true, subtypeCompactBoundary: true, subtypeThinkingTokens: true,
	subtypeTaskStarted: true, subtypeTaskNotification: true, subtypeTaskProgress: true, subtypeTaskUpdated: true,
}

// Parser normalizes Claude Code stream-json lines. It is an adapter.Runner;
// it keeps only the tool calls still waiting for their result and the
// background tasks still running.
type Parser struct {
	calls *openCalls
	// tasks maps the ID of each background task still running to the tool
	// call it runs for, which task_updated lines do not name.
	tasks *openCalls
}

// New returns a Parser for one attempt's log.
func New() *Parser { return &Parser{calls: newOpenCalls(), tasks: newOpenCalls()} }

// head is what every stream-json line may carry, decoded first.
type head struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Timestamp string `json:"timestamp"`
}

// Line implements adapter.Runner.
func (p *Parser) Line(text []byte, out *adapter.Emitter) {
	var h head
	if !adapter.DecodeObject(text, out, &h) {
		return
	}
	out.SetSource(h.Type, runnerTime(h.Timestamp))
	before := out.Len()
	if !p.render(h, text, out) || out.Len() == before {
		unrendered(h, out)
	}
}

// kindNamePattern is what a kind's name looks like: a name, not markup or
// prose, which a runner notice could not show as one.
var kindNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

// unrendered adds a line the parser did not render: a JSON line of a kind
// it does not know is a runner notice that names the kind, any other stays
// raw (as does one whose kind has no such name).
func unrendered(h head, out *adapter.Emitter) {
	reason := rawReason(h)
	if reason == contract.RawUnknownType && kindNamePattern.MatchString(kindName(h)) {
		out.Emit(noticeEvent(kindName(h), notice{}))
		return
	}
	out.RawLine(reason)
}

// render adds the events of a decoded line and reports whether it could:
// the type is known and the line has the shape the parser expects. It adds
// nothing when it cannot, and a line that produced nothing is kept raw.
func (p *Parser) render(h head, text []byte, out *adapter.Emitter) bool {
	if read, ok := typeNotices[h.Type]; ok {
		return renderNotice(h.Type, read, text, out)
	}
	switch h.Type {
	case typeSystem:
		return p.renderSystem(h.Subtype, text, out)
	case typeAssistant:
		return p.renderAssistant(text, out)
	case typeUser:
		return p.renderUser(text, out)
	case typeResult:
		return renderResult(text, out)
	case typeRateLimit:
		return renderRateLimit(text, out)
	case typeToolProgress:
		return renderToolProgress(text, out)
	}
	return false
}

// rawReason says why a line was not rendered: a type or system subtype the
// parser does not know, or a known one it does not render or whose shape it
// did not expect.
func rawReason(h head) contract.RawReason {
	switch h.Type {
	case typeSystem:
		if !renderedSubtypes[h.Subtype] {
			return contract.RawUnknownType
		}
		return contract.RawUnsupported
	case typeAssistant, typeUser, typeResult, typeRateLimit, typeToolProgress:
		return contract.RawUnsupported
	}
	return contract.RawUnknownType
}

// runnerTime is the line's own timestamp, nil when it has none or one that
// is not RFC 3339.
func runnerTime(s string) *contract.Timestamp {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return &contract.Timestamp{Time: t.UTC()}
}

// systemLine holds the fields of the system subtypes the parser renders.
type systemLine struct {
	SessionID       string          `json:"session_id"`
	Model           string          `json:"model"`
	Version         string          `json:"claude_code_version"`
	Attempt         *int            `json:"attempt"`
	MaxRetries      *int            `json:"max_retries"`
	RetryDelayMs    *int64          `json:"retry_delay_ms"`
	ErrorStatus     *int            `json:"error_status"`
	Error           string          `json:"error"`
	CompactMetadata json.RawMessage `json:"compact_metadata"`
}

// renderSystem renders the system subtypes the adapter knows; any other
// subtype is left to unrendered.
func (p *Parser) renderSystem(subtype string, text []byte, out *adapter.Emitter) bool {
	if read, ok := systemNotices[subtype]; ok {
		return renderNotice(subtype, read, text, out)
	}
	switch subtype {
	case subtypeTaskStarted, subtypeTaskProgress, subtypeTaskNotification:
		return p.renderTaskProgress(subtype, text, out)
	case subtypeTaskUpdated:
		return p.renderTaskUpdated(text, out)
	}
	var s systemLine
	if !renderedSubtypes[subtype] || json.Unmarshal(text, &s) != nil {
		return false
	}
	switch subtype {
	case subtypeInit:
		out.Emit(sessionStarted(s))
	case subtypeAPIRetry:
		out.Emit(apiRetry(s))
	case subtypeThinkingTokens:
		out.Emit(thinking(s))
	default:
		out.Emit(compacted(s.CompactMetadata))
	}
	return true
}

func sessionStarted(s systemLine) contract.Event {
	var cut adapter.Truncation
	ev := lifecycle(contract.LifecycleSessionStarted)
	ev.Lifecycle.SessionID = cut.Text("sessionId", s.SessionID, contract.MaxLabelBytes)
	ev.Lifecycle.Model = cut.Text("model", s.Model, contract.MaxLabelBytes)
	ev.Lifecycle.RunnerVersion = cut.Text("runnerVersion", s.Version, contract.MaxLabelBytes)
	ev.Truncated = cut.Fields()
	return ev
}

// thinking marks the model thinking: the CLI prints thinking_tokens lines
// while a thinking block streams, its estimated tokens left in the source.
func thinking(s systemLine) contract.Event {
	var cut adapter.Truncation
	ev := lifecycle(contract.LifecycleThinking)
	ev.Lifecycle.SessionID = cut.Text("sessionId", s.SessionID, contract.MaxLabelBytes)
	ev.Truncated = cut.Fields()
	return ev
}

// apiRetry is the error the CLI is about to retry; error_status is null
// when no response came.
func apiRetry(s systemLine) contract.Event {
	var cut adapter.Truncation
	info := &contract.ErrorInfo{
		Message: cut.Text("message", s.Error, contract.MaxTextBytes),
		Status:  adapter.HTTPStatus(s.ErrorStatus),
	}
	if s.Attempt != nil && adapter.ValidRetryCount(*s.Attempt) {
		info.Retry = &contract.Retry{Attempt: *s.Attempt, DelayMs: adapter.Count(s.RetryDelayMs)}
		if s.MaxRetries != nil && adapter.ValidRetryCount(*s.MaxRetries) {
			info.Retry.Max = s.MaxRetries
		}
	}
	return contract.Event{Kind: contract.KindError, Error: info, Truncated: cut.Fields()}
}

// compacted marks a context compaction; its metadata (trigger, tokens
// before) is the detail, as written.
func compacted(metadata json.RawMessage) contract.Event {
	ev := lifecycle(contract.LifecycleCompacted)
	if detail, ok := adapter.CompactJSON(metadata); ok {
		var cut adapter.Truncation
		ev.Lifecycle.Detail = cut.Optional("detail", &detail, contract.MaxTextBytes)
		ev.Truncated = cut.Fields()
	}
	return ev
}

func lifecycle(phase contract.LifecyclePhase) contract.Event {
	return contract.Event{Kind: contract.KindLifecycle, Lifecycle: &contract.Lifecycle{Phase: phase}}
}
