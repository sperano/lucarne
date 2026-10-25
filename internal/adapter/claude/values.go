package claude

import (
	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// maxRetryCount bounds a retry's attempt and max; a value outside it is
// dropped, not clamped.
const maxRetryCount = 999999

// apiUsage is an API usage object, on assistant messages and results.
type apiUsage struct {
	InputTokens   *int64 `json:"input_tokens"`
	OutputTokens  *int64 `json:"output_tokens"`
	CacheCreation *int64 `json:"cache_creation_input_tokens"`
	CacheRead     *int64 `json:"cache_read_input_tokens"`
}

// usage holds the token counters u reported; ok is false when it reported
// none.
func (u *apiUsage) usage(scope contract.UsageScope) (contract.Usage, bool) {
	if u == nil {
		return contract.Usage{}, false
	}
	usage := contract.Usage{
		Scope:            scope,
		InputTokens:      adapter.Count(u.InputTokens),
		OutputTokens:     adapter.Count(u.OutputTokens),
		CacheReadTokens:  adapter.Count(u.CacheRead),
		CacheWriteTokens: adapter.Count(u.CacheCreation),
	}
	return usage, usage.InputTokens != nil || usage.OutputTokens != nil ||
		usage.CacheReadTokens != nil || usage.CacheWriteTokens != nil
}

func validRetryCount(n int) bool { return n >= 1 && n <= maxRetryCount }
