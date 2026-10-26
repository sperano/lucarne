package opencode

import (
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// namedError is OpenCode's error shape (session.ts namedError): a name,
// such as APIError or ProviderAuthError, and data whose message most names
// carry. APIError also has the HTTP status and whether the provider call
// may be retried, which is not a retry OpenCode announced.
type namedError struct {
	Name string `json:"name"`
	Data *struct {
		Message    *string `json:"message"`
		StatusCode *int    `json:"statusCode"`
	} `json:"data"`
}

// renderError adds the error of an error line: a session.error, or the
// error the prompt request itself returned. A shape other than a named
// error keeps its JSON as the message.
func renderError(raw json.RawMessage, out *adapter.Emitter) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	info, ok := errorInfo(raw)
	if !ok {
		return false
	}
	var cut adapter.Truncation
	info.Message = cut.Text("message", info.Message, contract.MaxTextBytes)
	info.Code = cut.Text("code", info.Code, contract.MaxLabelBytes)
	out.Emit(contract.Event{Kind: contract.KindError, Error: &info, Truncated: cut.Fields()})
	return true
}

// errorInfo reads a named error, a string, or keeps any other JSON as the
// message, as run.ts does when it prints it without --format json.
func errorInfo(raw json.RawMessage) (contract.ErrorInfo, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return contract.ErrorInfo{Message: s}, true
	}
	var named namedError
	if isObject(raw) && json.Unmarshal(raw, &named) == nil && named.Name != "" {
		info := contract.ErrorInfo{Message: named.Name, Code: named.Name}
		if named.Data != nil {
			if named.Data.Message != nil && *named.Data.Message != "" {
				info.Message = *named.Data.Message
			}
			info.Status = adapter.HTTPStatus(named.Data.StatusCode)
		}
		return info, true
	}
	text, ok := adapter.CompactJSON(raw)
	return contract.ErrorInfo{Message: text}, ok
}
