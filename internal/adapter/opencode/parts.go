package opencode

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Part types (packages/schema/src/v1/session.ts), each emitted as the line
// type of the same name.
const (
	partText       = "text"
	partReasoning  = "reasoning"
	partTool       = "tool"
	partStepStart  = "step-start"
	partStepFinish = "step-finish"
)

// part is any part a line carries; each type sets its own fields.
type part struct {
	Type      string `json:"type"`
	MessageID string `json:"messageID"`
	// Text is a text or reasoning part's text.
	Text *string `json:"text"`
	// Synthetic marks text OpenCode itself wrote into a user message.
	Synthetic bool `json:"synthetic"`
	// Reason, Cost and Tokens are a step-finish part's.
	Reason *string      `json:"reason"`
	Cost   *json.Number `json:"cost"`
	Tokens *tokens      `json:"tokens"`
	// CallID, Tool and State are a tool part's.
	CallID string     `json:"callID"`
	Tool   string     `json:"tool"`
	State  *toolState `json:"state"`
}

// tokens is a step's token counts. OpenCode types them as numbers, so a
// count that is not a non-negative integer is dropped.
type tokens struct {
	Total     *json.Number `json:"total"`
	Input     *json.Number `json:"input"`
	Output    *json.Number `json:"output"`
	Reasoning *json.Number `json:"reasoning"`
	Cache     *struct {
		Read  *json.Number `json:"read"`
		Write *json.Number `json:"write"`
	} `json:"cache"`
}

// renderText adds a text or reasoning part as a message. run.ts prints a
// text part only once it has ended, so the text is whole. A synthetic part
// is text OpenCode wrote into a user message (the prompt after a
// compaction); every other text part of 1.19.0 that has ended is the
// assistant's.
func renderText(p part, kind contract.MessageType, partType string, out *adapter.Emitter) bool {
	if p.Type != partType || p.Text == nil {
		return false
	}
	role := contract.RoleAssistant
	if p.Synthetic {
		role = contract.RoleUser
	}
	var cut adapter.Truncation
	m := &contract.Message{
		Role:      role,
		Type:      kind,
		Text:      cut.Text("text", *p.Text, contract.MaxTextBytes),
		MessageID: cut.Text("messageId", p.MessageID, contract.MaxLabelBytes),
	}
	out.Emit(contract.Event{Kind: contract.KindMessage, Message: m, Truncated: cut.Fields()})
	return true
}

// renderStepStart marks the start of a model call.
func renderStepStart(l line, out *adapter.Emitter) bool {
	if l.Part.Type != partStepStart {
		return false
	}
	out.Emit(step(contract.LifecycleStepStarted, l.SessionID, ""))
	return true
}

// renderStepFinish adds the end of a model call with its finish reason, and
// the call's usage when it reported any.
func renderStepFinish(l line, out *adapter.Emitter) bool {
	p := l.Part
	if p.Type != partStepFinish {
		return false
	}
	reason := ""
	if p.Reason != nil {
		reason = *p.Reason
	}
	out.Emit(step(contract.LifecycleStepCompleted, l.SessionID, reason))
	if usage, ok := stepUsage(p); ok {
		out.Emit(contract.Event{Kind: contract.KindUsage, Usage: &usage})
	}
	return true
}

func step(phase contract.LifecyclePhase, sessionID, reason string) contract.Event {
	var cut adapter.Truncation
	ev := lifecycle(phase)
	ev.Lifecycle.SessionID = cut.Text("sessionId", sessionID, contract.MaxLabelBytes)
	ev.Lifecycle.StopReason = cut.Text("stopReason", reason, contract.MaxLabelBytes)
	ev.Truncated = cut.Fields()
	return ev
}

// stepUsage is a step's tokens and cost; ok is false when it reported
// none.
func stepUsage(p *part) (contract.Usage, bool) {
	usage := contract.Usage{Scope: contract.ScopeStep}
	if t := p.Tokens; t != nil {
		usage.InputTokens = counter(t.Input)
		usage.OutputTokens = counter(t.Output)
		usage.ReasoningTokens = counter(t.Reasoning)
		usage.TotalTokens = counter(t.Total)
		if t.Cache != nil {
			usage.CacheReadTokens = counter(t.Cache.Read)
			usage.CacheWriteTokens = counter(t.Cache.Write)
		}
	}
	if p.Cost != nil {
		usage.CostUSD = adapter.CostUSD(*p.Cost)
	}
	return usage, usage.InputTokens != nil || usage.OutputTokens != nil ||
		usage.ReasoningTokens != nil || usage.TotalTokens != nil ||
		usage.CacheReadTokens != nil || usage.CacheWriteTokens != nil ||
		usage.CostUSD != ""
}

// counter is n when it is a non-negative integer, nil otherwise.
func counter(n *json.Number) *int64 {
	if n == nil {
		return nil
	}
	i, err := n.Int64()
	if err != nil {
		return nil
	}
	return adapter.Count(&i)
}
