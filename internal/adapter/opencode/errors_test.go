package opencode

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestErrorShapes(t *testing.T) {
	t.Parallel()
	status := func(n int) *int { return &n }
	cases := []struct {
		name  string
		error string
		want  contract.ErrorInfo
	}{
		{"api error", `{"name":"APIError","data":{"message":"Overloaded","statusCode":529,"isRetryable":true}}`,
			contract.ErrorInfo{Message: "Overloaded", Code: "APIError", Status: status(529)}},
		{"status out of range", `{"name":"APIError","data":{"message":"x","statusCode":700}}`,
			contract.ErrorInfo{Message: "x", Code: "APIError"}},
		{"no message", `{"name":"MessageOutputLengthError","data":{}}`,
			contract.ErrorInfo{Message: "MessageOutputLengthError", Code: "MessageOutputLengthError"}},
		{"no data", `{"name":"UnknownError"}`, contract.ErrorInfo{Message: "UnknownError", Code: "UnknownError"}},
		{"string", `"example failure"`, contract.ErrorInfo{Message: "example failure"}},
		{"other object", `{"success":false, "errors":[{"message":"bad"}]}`,
			contract.ErrorInfo{Message: `{"success":false,"errors":[{"message":"bad"}]}`}},
		{"message not a string", `{"name":"APIError","data":{"message":7}}`,
			contract.ErrorInfo{Message: `{"name":"APIError","data":{"message":7}}`}},
		{"number", `7`, contract.ErrorInfo{Message: "7"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := only(t, normalize(t, `{"type":"error","error":`+c.error+`}`), contract.KindError).Error
			if got.Message != c.want.Message || got.Code != c.want.Code || got.Retry != nil ||
				(got.Status == nil) != (c.want.Status == nil) || (got.Status != nil && *got.Status != *c.want.Status) {
				t.Errorf("error = %+v, want %+v", got, c.want)
			}
		})
	}
}
