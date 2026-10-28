package claude

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

type rateLimitLine struct {
	SessionID string          `json:"session_id"`
	Info      json.RawMessage `json:"rate_limit_info"`
}

type rateLimitInfo struct {
	Status string `json:"status"`
}

// renderRateLimit keeps the runner's complete rate-limit information as the
// detail while promoting its status and session to lifecycle fields.
func renderRateLimit(text []byte, out *adapter.Emitter) bool {
	var line rateLimitLine
	var info rateLimitInfo
	if json.Unmarshal(text, &line) != nil || !isObject(line.Info) || json.Unmarshal(line.Info, &info) != nil || info.Status == "" {
		return false
	}
	detail, ok := adapter.CompactJSON(line.Info)
	if !ok {
		return false
	}
	var cut adapter.Truncation
	ev := lifecycle(contract.LifecycleRateLimitStatus)
	ev.Lifecycle.SessionID = cut.Text("sessionId", line.SessionID, contract.MaxLabelBytes)
	ev.Lifecycle.Subtype = cut.Text("subtype", info.Status, contract.MaxLabelBytes)
	ev.Lifecycle.Detail = cut.Optional("detail", &detail, contract.MaxTextBytes)
	ev.Truncated = cut.Fields()
	out.Emit(ev)
	return true
}
