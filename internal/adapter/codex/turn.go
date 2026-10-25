package codex

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Ranges the contract accepts; a value outside them is dropped, not
// clamped.
const (
	// maxCount is the schema's Count maximum, JSON's largest safe integer.
	maxCount = 1<<53 - 1
	// maxRetryCount bounds a retry's attempt and max.
	maxRetryCount = 999999
)

// reconnecting is how Codex words a stream retry it is about to make
// (codex-rs/core/src/responses_retry.rs): "Reconnecting... n/max", then
// the cause in parentheses. Each number is at most as wide as
// maxRetryCount, so a longer one is not a retry the contract accepts.
var reconnecting = regexp.MustCompile(fmt.Sprintf(
	`^Reconnecting\.\.\. ([0-9]{1,%[1]d})/([0-9]{1,%[1]d})(?: |$)`,
	len(strconv.Itoa(maxRetryCount))))

// tokenUsage is turn.completed's usage: the thread's running total
// (last_total_token_usage), all zeros when Codex received no token count.
type tokenUsage struct {
	InputTokens     *int64 `json:"input_tokens"`
	CachedInput     *int64 `json:"cached_input_tokens"`
	CacheWriteInput *int64 `json:"cache_write_input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	ReasoningTokens *int64 `json:"reasoning_output_tokens"`
}

// renderTurn renders thread.started, turn.started, turn.completed and
// turn.failed.
func renderTurn(line threadLine, out *adapter.Emitter) contract.RawReason {
	switch line.Type {
	case typeThreadStarted:
		if line.ThreadID == nil {
			return contract.RawUnsupported
		}
		var cut adapter.Truncation
		ev := lifecycle(contract.LifecycleSessionStarted)
		ev.Lifecycle.SessionID = cut.Text("sessionId", *line.ThreadID, contract.MaxLabelBytes)
		ev.Truncated = cut.Fields()
		out.Emit(ev)
	case typeTurnStarted:
		out.Emit(lifecycle(contract.LifecycleTurnStarted))
	case typeTurnCompleted:
		out.Emit(lifecycle(contract.LifecycleTurnCompleted))
		if usage, ok := line.Usage.usage(); ok {
			out.Emit(contract.Event{Kind: contract.KindUsage, Usage: &usage})
		}
	case typeTurnFailed:
		out.Emit(turnFailed(line.Error))
	default:
		return contract.RawUnsupported
	}
	return ""
}

// turnFailed carries the turn's error as its detail: Codex prints the same
// message as an error line first when the failure came from the stream.
func turnFailed(e *errorPayload) contract.Event {
	ev := lifecycle(contract.LifecycleTurnFailed)
	if e != nil && e.Message != nil {
		var cut adapter.Truncation
		ev.Lifecycle.Detail = cut.Optional("detail", e.Message, contract.MaxTextBytes)
		ev.Truncated = cut.Fields()
	}
	return ev
}

// renderError renders a top-level error: a server error, or a stream retry
// with its attempt and maximum.
func renderError(message *string, out *adapter.Emitter) contract.RawReason {
	if message == nil {
		return contract.RawUnsupported
	}
	var cut adapter.Truncation
	info := &contract.ErrorInfo{
		Message: cut.Text("message", *message, contract.MaxTextBytes),
		Retry:   announcedRetry(*message),
	}
	out.Emit(contract.Event{Kind: contract.KindError, Error: info, Truncated: cut.Fields()})
	return ""
}

// announcedRetry is the retry a "Reconnecting... n/max" message announces,
// nil for any other message.
func announcedRetry(message string) *contract.Retry {
	m := reconnecting.FindStringSubmatch(message)
	if m == nil {
		return nil
	}
	// Atoi cannot fail: reconnecting caps each number at maxRetryCount's
	// width, far below int's range. Widening the regex breaks this.
	attempt, _ := strconv.Atoi(m[1])
	limit, _ := strconv.Atoi(m[2])
	if !validRetryCount(attempt) {
		return nil
	}
	retry := &contract.Retry{Attempt: attempt}
	if validRetryCount(limit) {
		retry.Max = &limit
	}
	return retry
}

func validRetryCount(n int) bool { return n >= 1 && n <= maxRetryCount }

// usage holds the counters u reported; ok is false when it reported none,
// or only zeros, which is what Codex prints when it received no count.
func (u *tokenUsage) usage() (contract.Usage, bool) {
	if u == nil {
		return contract.Usage{}, false
	}
	usage := contract.Usage{
		Scope:            contract.ScopeSession,
		InputTokens:      count(u.InputTokens),
		CacheReadTokens:  count(u.CachedInput),
		CacheWriteTokens: count(u.CacheWriteInput),
		OutputTokens:     count(u.OutputTokens),
		ReasoningTokens:  count(u.ReasoningTokens),
	}
	for _, n := range []*int64{usage.InputTokens, usage.CacheReadTokens, usage.CacheWriteTokens, usage.OutputTokens, usage.ReasoningTokens} {
		if n != nil && *n > 0 {
			return usage, true
		}
	}
	return contract.Usage{}, false
}

// count is n when it is a valid counter, nil otherwise.
func count(n *int64) *int64 {
	if n == nil || *n < 0 || *n > maxCount {
		return nil
	}
	return n
}

func lifecycle(phase contract.LifecyclePhase) contract.Event {
	return contract.Event{Kind: contract.KindLifecycle, Lifecycle: &contract.Lifecycle{Phase: phase}}
}
