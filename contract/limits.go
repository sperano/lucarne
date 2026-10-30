package contract

// Input limits bound what the server reads from a source.
const (
	// MaxLineBytes is the longest log line parsed; a longer one becomes a
	// raw "oversized" event holding its first MaxRawBytes.
	MaxLineBytes = 1 << 20
	// MaxAttemptLogBytes is the most log read for one attempt; the rest is
	// an "input_limit" gap.
	MaxAttemptLogBytes = 64 << 20
)

// Output limits bound what one event, run or stream carries. Text is cut at
// a UTF-8 boundary and the field named in the event's "truncated" list.
const (
	MaxTextBytes       = 64 << 10
	MaxToolInputBytes  = 16 << 10
	MaxToolOutputBytes = 32 << 10
	MaxDiffBytes       = 64 << 10
	MaxRawBytes        = 8 << 10
	MaxPathBytes       = 4096
	// MaxLabelBytes bounds names, identifiers, codes and reasons.
	MaxLabelBytes = 256
	// MaxEventsPerRun and MaxRunBytes bound the events a run retains; the
	// oldest are dropped first and counted in Stream.DroppedEvents.
	MaxEventsPerRun = 20000
	MaxRunBytes     = 32 << 20
	// MaxAttemptsPerRun bounds the attempts a run lists.
	MaxAttemptsPerRun = 16
	// MaxReplayUpdates is how many stream updates a reconnecting client can
	// be replayed; an older cursor gets a "replay_expired" reset.
	MaxReplayUpdates = 5000
	// MaxClientQueue is how many updates wait for one slow SSE client
	// before it gets a "client_too_slow" reset.
	MaxClientQueue = 1024
	// MaxOutputs bounds the entries of a kelos_outputs lifecycle event.
	MaxOutputs = 64
	// MaxMedia bounds the media entries of a tool result.
	MaxMedia = 64
	// MaxTruncatedFields bounds an event's "truncated" list.
	MaxTruncatedFields = 8
	// MaxSourceBytes bounds the text of a SourceLine: the start of a log
	// line, as the runner wrote it, that the inspector shows next to its
	// events.
	MaxSourceBytes = 64 << 10
	// MaxRunSourceBytes bounds the source lines a run retains; the oldest
	// are dropped first, and a request for one of them is answered
	// "source_not_found".
	MaxRunSourceBytes = 16 << 20
	// MaxListedRuns bounds the runs a RunList holds: the newest are
	// listed, the older ones counted in RunList.OmittedRuns.
	MaxListedRuns = 1000
)

// Limits is the set of limits a server enforces, published in every
// snapshot so the UI can say what was cut.
type Limits struct {
	MaxLineBytes       int64 `json:"maxLineBytes"`
	MaxAttemptLogBytes int64 `json:"maxAttemptLogBytes"`
	MaxTextBytes       int64 `json:"maxTextBytes"`
	MaxToolInputBytes  int64 `json:"maxToolInputBytes"`
	MaxToolOutputBytes int64 `json:"maxToolOutputBytes"`
	MaxDiffBytes       int64 `json:"maxDiffBytes"`
	MaxRawBytes        int64 `json:"maxRawBytes"`
	MaxPathBytes       int64 `json:"maxPathBytes"`
	MaxLabelBytes      int64 `json:"maxLabelBytes"`
	MaxEventsPerRun    int64 `json:"maxEventsPerRun"`
	MaxRunBytes        int64 `json:"maxRunBytes"`
	MaxAttemptsPerRun  int64 `json:"maxAttemptsPerRun"`
	MaxReplayUpdates   int64 `json:"maxReplayUpdates"`
	MaxClientQueue     int64 `json:"maxClientQueue"`
	MaxSourceBytes     int64 `json:"maxSourceBytes"`
	MaxRunSourceBytes  int64 `json:"maxRunSourceBytes"`
	MaxListedRuns      int64 `json:"maxListedRuns"`
}

// DefaultLimits returns the limits of this contract version.
func DefaultLimits() Limits {
	return Limits{
		MaxLineBytes:       MaxLineBytes,
		MaxAttemptLogBytes: MaxAttemptLogBytes,
		MaxTextBytes:       MaxTextBytes,
		MaxToolInputBytes:  MaxToolInputBytes,
		MaxToolOutputBytes: MaxToolOutputBytes,
		MaxDiffBytes:       MaxDiffBytes,
		MaxRawBytes:        MaxRawBytes,
		MaxPathBytes:       MaxPathBytes,
		MaxLabelBytes:      MaxLabelBytes,
		MaxEventsPerRun:    MaxEventsPerRun,
		MaxRunBytes:        MaxRunBytes,
		MaxAttemptsPerRun:  MaxAttemptsPerRun,
		MaxReplayUpdates:   MaxReplayUpdates,
		MaxClientQueue:     MaxClientQueue,
		MaxSourceBytes:     MaxSourceBytes,
		MaxRunSourceBytes:  MaxRunSourceBytes,
		MaxListedRuns:      MaxListedRuns,
	}
}
