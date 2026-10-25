package contract

import (
	"encoding/json"
	"fmt"
	"time"
)

// SchemaVersion names this contract version in snapshots and ContractInfo.
const SchemaVersion = "lucarne.transcript/v1"

// APIPrefix is the path prefix of every API endpoint of this version.
const APIPrefix = "/api/v1"

// ContractPath is the API path of ContractInfo.
const ContractPath = APIPrefix + "/contract"

// SSE event names on the run stream.
const (
	SSETranscript = "transcript"
	SSEAttempt    = "attempt"
	SSERun        = "run"
	SSEReset      = "reset"
	SSEEnd        = "end"
)

// AttemptUpdate is the data of an "attempt" SSE event.
type AttemptUpdate struct {
	Seq     int64   `json:"seq"`
	Attempt Attempt `json:"attempt"`
}

// RunUpdate is the data of a "run" SSE event.
type RunUpdate struct {
	Seq int64 `json:"seq"`
	Run Run   `json:"run"`
}

// Reset is the data of a "reset" SSE event: the client must drop its state
// and load a new snapshot.
type Reset struct {
	Epoch  string      `json:"epoch"`
	Reason ResetReason `json:"reason"`
}

// ResetReason says why the stream was reset.
type ResetReason string

// Reset reasons; see contract/README.md.
const (
	ResetEpochChanged  ResetReason = "epoch_changed"
	ResetReplayExpired ResetReason = "replay_expired"
	ResetClientTooSlow ResetReason = "client_too_slow"
	ResetBadCursor     ResetReason = "bad_cursor"
)

// End is the data of an "end" SSE event: nothing more will follow.
type End struct {
	Seq    int64     `json:"seq"`
	Reason EndReason `json:"reason"`
}

// EndReason says why the stream ended.
type EndReason string

// End reasons. RunFinished needs a terminal run phase (succeeded or failed)
// and every attempt ended or expired: a run whose attempts all ended while
// a retry starts is not finished.
const (
	EndRunFinished EndReason = "run_finished"
	EndRunDeleted  EndReason = "run_deleted"
)

// Error codes of ErrorResponse.
const (
	ErrorInvalidRunKey  = "invalid_run_key"
	ErrorRunNotFound    = "run_not_found"
	ErrorNotImplemented = "not_implemented"
	ErrorInternal       = "internal"
	// ErrorNotFound is an API path that names no endpoint.
	ErrorNotFound = "not_found"
	// ErrorMethodNotAllowed is a request with another method than GET.
	ErrorMethodNotAllowed = "method_not_allowed"
)

// ErrorResponse is the body of every API error.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is an API error's code and human-readable message.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ContractInfo is GET /api/v1/contract.
type ContractInfo struct {
	Schema string `json:"schema"`
	Limits Limits `json:"limits"`
}

// Timestamp is a time serialized in UTC, RFC 3339 with up to nanoseconds.
type Timestamp struct{ time.Time }

// MarshalJSON writes the time in UTC whatever its location.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339Nano))
}

// UnmarshalJSON accepts only UTC ("Z") times, as the schema does.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	if _, offset := parsed.Zone(); offset != 0 || s[len(s)-1] != 'Z' {
		return fmt.Errorf("time %q is not in UTC", s)
	}
	t.Time = parsed.UTC()
	return nil
}
