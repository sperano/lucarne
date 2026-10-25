package claude

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// resultLine is the session's last line. Its result text repeats the last
// assistant message and is not rendered again.
type resultLine struct {
	Subtype        string            `json:"subtype"`
	IsError        *bool             `json:"is_error"`
	StopReason     string            `json:"stop_reason"`
	DurationMs     *int64            `json:"duration_ms"`
	DurationAPIMs  *int64            `json:"duration_api_ms"`
	NumTurns       *int64            `json:"num_turns"`
	TotalCostUSD   json.Number       `json:"total_cost_usd"`
	Usage          *apiUsage         `json:"usage"`
	Result         *string           `json:"result"`
	APIErrorStatus *int              `json:"api_error_status"`
	Errors         []json.RawMessage `json:"errors"`
}

// renderResult adds the session's completion, its usage, and the errors it
// reported.
func renderResult(text []byte, out *adapter.Emitter) bool {
	var r resultLine
	if json.Unmarshal(text, &r) != nil {
		return false
	}
	out.Emit(sessionCompleted(r))
	if usage, ok := sessionUsage(r); ok {
		out.Emit(contract.Event{Kind: contract.KindUsage, Usage: &usage})
	}
	for _, ev := range resultErrors(r) {
		out.Emit(ev)
	}
	return true
}

func sessionCompleted(r resultLine) contract.Event {
	var cut adapter.Truncation
	ev := lifecycle(contract.LifecycleSessionCompleted)
	ev.Lifecycle.Subtype = cut.Text("subtype", r.Subtype, contract.MaxLabelBytes)
	ev.Lifecycle.IsError = r.IsError
	ev.Lifecycle.StopReason = cut.Text("stopReason", r.StopReason, contract.MaxLabelBytes)
	ev.Truncated = cut.Fields()
	return ev
}

// sessionUsage is the session's totals: tokens, cost, wall and API time,
// turns; ok is false when the line reported none of them.
func sessionUsage(r resultLine) (contract.Usage, bool) {
	usage, ok := r.Usage.usage(contract.ScopeSession)
	if !ok {
		usage = contract.Usage{Scope: contract.ScopeSession}
	}
	usage.CostUSD = costUSD(r.TotalCostUSD)
	usage.DurationMs = count(r.DurationMs)
	usage.APIDurationMs = count(r.DurationAPIMs)
	usage.Turns = count(r.NumTurns)
	return usage, ok || usage.CostUSD != "" || usage.DurationMs != nil ||
		usage.APIDurationMs != nil || usage.Turns != nil
}

// resultErrors is one error per entry of errors, each with the API's HTTP
// status when the line has one. Without errors, a status is reported with
// the result text, the only message the line has for it.
func resultErrors(r resultLine) []contract.Event {
	status := httpStatus(r.APIErrorStatus)
	var events []contract.Event
	for _, raw := range r.Errors {
		events = append(events, resultError(errorText(raw), status))
	}
	if events == nil && status != nil {
		text := ""
		if r.Result != nil {
			text = *r.Result
		}
		events = append(events, resultError(text, status))
	}
	return events
}

func resultError(text string, status *int) contract.Event {
	var cut adapter.Truncation
	info := &contract.ErrorInfo{Message: cut.Text("message", text, contract.MaxTextBytes), Status: status}
	return contract.Event{Kind: contract.KindError, Error: info, Truncated: cut.Fields()}
}

// errorText is an entry of errors: its text when it is a string, else its
// JSON.
func errorText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	compact, _ := compactJSON(raw)
	return compact
}
