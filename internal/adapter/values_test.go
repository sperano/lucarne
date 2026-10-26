package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestCostUSD(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"0.0215": "0.0215",
		"0":      "0",
		"1e-7":   "0.0000001",
		"1.5E2":  "150",
		"-0.01":  "",
		"":       "",
		"1e-30":  "",
	}
	for in, want := range cases {
		if got := CostUSD(json.Number(in)); got != want {
			t.Errorf("CostUSD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountAndHTTPStatus(t *testing.T) {
	t.Parallel()
	n, neg, top, over := int64(3), int64(-1), int64(MaxCount), int64(MaxCount+1)
	if Count(&n) == nil || Count(&neg) != nil || Count(nil) != nil || Count(&top) == nil || Count(&over) != nil {
		t.Error("Count keeps only counters from 0 to MaxCount")
	}
	ok, low, high := 529, 99, 600
	if HTTPStatus(&ok) == nil || HTTPStatus(&low) != nil || HTTPStatus(&high) != nil || HTTPStatus(nil) != nil {
		t.Error("HTTPStatus keeps only HTTP status codes")
	}
}

func TestValidRetryCount(t *testing.T) {
	t.Parallel()
	if !ValidRetryCount(1) || !ValidRetryCount(MaxRetryCount) || ValidRetryCount(0) || ValidRetryCount(MaxRetryCount+1) {
		t.Error("ValidRetryCount keeps only counts from 1 to MaxRetryCount")
	}
}

func TestCompactJSON(t *testing.T) {
	t.Parallel()
	if got, ok := CompactJSON(json.RawMessage(`{ "a": [1, 2] }`)); !ok || got != `{"a":[1,2]}` {
		t.Errorf("CompactJSON = %q, %v", got, ok)
	}
	for _, raw := range []string{"", "null", "{"} {
		if _, ok := CompactJSON(json.RawMessage(raw)); ok {
			t.Errorf("CompactJSON(%q) is ok", raw)
		}
	}
}

func TestSetToolInput(t *testing.T) {
	t.Parallel()
	var call contract.ToolCall
	var cut Truncation
	SetToolInput(&call, json.RawMessage(`{"a": 1}`), &cut)
	if string(call.Input) != `{"a":1}` || call.InputText != "" || cut.Fields() != nil {
		t.Errorf("small input: %+v, truncated %v", call, cut.Fields())
	}
	call, cut = contract.ToolCall{}, Truncation{}
	SetToolInput(&call, json.RawMessage(`{"a":"`+strings.Repeat("x", contract.MaxToolInputBytes)+`"}`), &cut)
	if call.Input != nil || len(call.InputText) != contract.MaxToolInputBytes || cut.Fields()[0] != "inputText" {
		t.Errorf("large input: %d input bytes, %d text bytes, truncated %v", len(call.Input), len(call.InputText), cut.Fields())
	}
	call, cut = contract.ToolCall{}, Truncation{}
	SetToolInput(&call, nil, &cut)
	if call.Input != nil || call.InputText != "" {
		t.Errorf("absent input: %+v", call)
	}
}
