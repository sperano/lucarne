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

// RunsPath is the API path of RunList.
const RunsPath = APIPrefix + "/runs"

// SSE event names on the run stream.
const (
	SSETranscript = "transcript"
	SSEAttempt    = "attempt"
	SSERun        = "run"
	SSEReset      = "reset"
	SSEEnd        = "end"
	SSELive       = "live"
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

// Live is the data of a "live" SSE event, sent once after the updates a
// stream replayed: those up to LastSeq had happened before the client
// connected, the ones after it are new.
type Live struct {
	LastSeq int64 `json:"lastSeq"`
}

// ErrorCode is the machine-readable code of an API error.
type ErrorCode string

// Error codes of ErrorResponse.
const (
	// ErrorInvalidRunKey is a path under /api/v1/runs/ that is not one of the
	// two run paths, or whose run key has a percent-encoded character.
	ErrorInvalidRunKey ErrorCode = "invalid_run_key"
	// ErrorRunNotFound is a valid run key with no such run, or one that expired.
	ErrorRunNotFound ErrorCode = "run_not_found"
	// ErrorNotImplemented is an endpoint that exists but is not served yet.
	ErrorNotImplemented ErrorCode = "not_implemented"
	// ErrorInternal is a server-side failure.
	ErrorInternal ErrorCode = "internal"
	// ErrorNotFound is an API path that names no endpoint.
	ErrorNotFound ErrorCode = "not_found"
	// ErrorMethodNotAllowed is a request with another method than GET or HEAD.
	ErrorMethodNotAllowed ErrorCode = "method_not_allowed"
	// ErrorUnavailable is a run the server cannot read now: the Kubernetes
	// API did not answer or refused, or the server follows as many runs as
	// it can.
	ErrorUnavailable ErrorCode = "unavailable"
	// ErrorInvalidEventID is a source path whose event ID is not in
	// canonical form.
	ErrorInvalidEventID ErrorCode = "invalid_event_id"
	// ErrorSourceNotFound is an event ID whose line the run does not hold:
	// a gap, which no line produced, a line dropped past
	// MaxRunSourceBytes, or one of another run or read.
	ErrorSourceNotFound ErrorCode = "source_not_found"
)

// ErrorResponse is the body of every API error.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is an API error's code and human-readable message.
type ErrorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// SourceLine is GET /api/v1/runs/{cluster}/{namespace}/{taskUID}/source/{eventID}:
// the log line an event came from, as the runner wrote it, for the
// inspector to show next to what the adapter made of it.
type SourceLine struct {
	Schema  string `json:"schema"`
	Attempt string `json:"attempt"`
	Origin  string `json:"origin"`
	Line    int64  `json:"line"`
	// Text is the line without its timestamp prefix, cut to
	// MaxSourceBytes at a UTF-8 boundary.
	Text string `json:"text"`
	// Bytes is the length of the whole line: more than Text holds when it
	// was cut.
	Bytes int64 `json:"bytes"`
}

// RunList is GET /api/v1/runs: the runs of the allowlisted namespaces,
// newest first, for the UI to search and open. It holds at most
// MaxListedRuns; OmittedRuns counts the older ones left out.
type RunList struct {
	Schema      string `json:"schema"`
	Runs        []Run  `json:"runs"`
	OmittedRuns int64  `json:"omittedRuns"`
}

// MarshalJSON writes nil Runs as an empty array, which the schema requires.
func (l RunList) MarshalJSON() ([]byte, error) {
	type plain RunList
	if l.Runs == nil {
		l.Runs = []Run{}
	}
	return json.Marshal(plain(l))
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
