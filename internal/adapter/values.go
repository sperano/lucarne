package adapter

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
)

// decimalUSD is the schema's DecimalUSD pattern.
var decimalUSD = regexp.MustCompile(`^(0|[1-9][0-9]{0,15})(\.[0-9]{1,18})?$`)

// Count is n when it is a valid counter, nil otherwise.
func Count(n *int64) *int64 {
	if n == nil || *n < 0 {
		return nil
	}
	return n
}

// HTTPStatus is status when it is an HTTP status code, nil otherwise.
func HTTPStatus(status *int) *int {
	if status == nil || *status < minHTTPStatus || *status > maxHTTPStatus {
		return nil
	}
	return status
}

// CostUSD writes a JSON number as a plain decimal string: as written when
// it already is one, else in the shortest plain form. It is empty when the
// number is negative or has no such form within the schema's precision.
func CostUSD(n json.Number) string {
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

// CompactJSON is raw without insignificant space; ok is false when raw is
// absent or null.
func CompactJSON(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return "", false
	}
	return buf.String(), true
}

// SetToolInput sets call's input: the JSON when it fits
// contract.MaxToolInputBytes, else its cut serialization in InputText.
// Absent or null input sets neither.
func SetToolInput(call *contract.ToolCall, raw json.RawMessage, cut *Truncation) {
	input, ok := CompactJSON(raw)
	if !ok {
		return
	}
	cut.ToolInput(call, input)
}
