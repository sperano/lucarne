package claude

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/sperano/lucarne/contract"
)

// Ranges the contract accepts; a value outside them is dropped, not
// clamped.
const (
	minHTTPStatus = 100
	maxHTTPStatus = 599
	// maxRetryCount bounds a retry's attempt and max.
	maxRetryCount = 999999
)

// decimalUSD is the schema's DecimalUSD pattern.
var decimalUSD = regexp.MustCompile(`^(0|[1-9][0-9]{0,15})(\.[0-9]{1,18})?$`)

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
		InputTokens:      count(u.InputTokens),
		OutputTokens:     count(u.OutputTokens),
		CacheReadTokens:  count(u.CacheRead),
		CacheWriteTokens: count(u.CacheCreation),
	}
	return usage, usage.InputTokens != nil || usage.OutputTokens != nil ||
		usage.CacheReadTokens != nil || usage.CacheWriteTokens != nil
}

// count is n when it is a valid counter, nil otherwise.
func count(n *int64) *int64 {
	if n == nil || *n < 0 {
		return nil
	}
	return n
}

func httpStatus(status *int) *int {
	if status == nil || *status < minHTTPStatus || *status > maxHTTPStatus {
		return nil
	}
	return status
}

func validRetryCount(n int) bool { return n >= 1 && n <= maxRetryCount }

// costUSD writes a JSON number as a plain decimal string: as written when
// it already is one, else in the shortest plain form. It is empty when the
// number is negative or has no such form within the schema's precision.
func costUSD(n json.Number) string {
	s := n.String()
	if s == "" || decimalUSD.MatchString(s) {
		return s
	}
	f, err := n.Float64()
	if err != nil {
		return ""
	}
	if plain := strconv.FormatFloat(f, 'f', -1, 64); decimalUSD.MatchString(plain) {
		return plain
	}
	return ""
}

// compactJSON is raw without insignificant space; ok is false when raw is
// absent or null.
func compactJSON(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return "", false
	}
	return buf.String(), true
}
