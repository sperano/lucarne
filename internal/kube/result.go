package kube

import (
	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Coverage reasons, as the UI shows them.
const (
	ReasonNotStarted        = "container not started"
	ReasonNeverStarted      = "container never started"
	ReasonOlderRestart      = "only the current and previous instance of a container keep their log"
	ReasonInputLimit        = "log longer than maxAttemptLogBytes"
	ReasonPodDeleted        = "pod deleted"
	ReasonRunDeleted        = "task deleted"
	ReasonForbidden         = "forbidden: the server may not read this log"
	ReasonSourceUnavailable = "source unavailable: the Kubernetes API did not answer"
	ReasonCanceled          = "read canceled"
	// ReasonNoResume is a follow that broke after a line without a
	// timestamp, which leaves nothing to resume from.
	ReasonNoResume = "log stream broke and its last line has no timestamp to resume from"
)

// ReadEnd says why a log read stopped.
type ReadEnd string

// Read ends.
const (
	// EndPending is an instance that has not started: there is no log yet.
	EndPending ReadEnd = "pending"
	// EndComplete is a terminated instance whose log was read to its end.
	EndComplete ReadEnd = "complete"
	// EndCurrent is a snapshot that reached the current end of the log of
	// an instance still running: more can follow.
	EndCurrent ReadEnd = "current"
	// EndLimit is a log that passed contract.MaxAttemptLogBytes.
	EndLimit ReadEnd = "limit"
	// EndExpired is a log that is gone: the pod, the Task, an older restart.
	EndExpired ReadEnd = "expired"
	// EndUnavailable is a log that exists but could not be read.
	EndUnavailable ReadEnd = "unavailable"
	// EndCanceled is a read whose context was canceled.
	EndCanceled ReadEnd = "canceled"
)

// ReadResult is what one log read did. Lines counts the lines read, blank
// ones included, Gaps the gaps reported, TruncatedLines the lines longer
// than contract.MaxLineBytes, Bytes the log read, timestamps and line
// endings included. End is empty when ReadLog returned an error other than
// the context's.
type ReadResult struct {
	Origin         string
	Lines          int64
	Gaps           int64
	TruncatedLines int64
	Bytes          int64
	End            ReadEnd
	Reason         string
}

// coverageStates maps each read end to its coverage state. A snapshot of a
// running instance is following: the reader caught up, more is coming.
var coverageStates = map[ReadEnd]contract.CoverageState{
	EndPending:     contract.CoveragePending,
	EndComplete:    contract.CoverageEnded,
	EndCurrent:     contract.CoverageFollowing,
	EndLimit:       contract.CoverageEnded,
	EndExpired:     contract.CoverageExpired,
	EndUnavailable: contract.CoverageUnavailable,
	EndCanceled:    contract.CoverageUnavailable,
}

// Coverage is the attempt's coverage after this read.
func (r ReadResult) Coverage() contract.Coverage {
	return contract.Coverage{
		State:          coverageStates[r.End],
		Lines:          r.Lines,
		Gaps:           r.Gaps,
		TruncatedLines: r.TruncatedLines,
		Reason:         cutLabel(r.Reason),
	}
}

func pending(reason string) contract.Coverage {
	return contract.Coverage{State: contract.CoveragePending, Reason: reason}
}

func expired(reason string) contract.Coverage {
	return contract.Coverage{State: contract.CoverageExpired, Reason: reason}
}

// cutLabel bounds a reason or name to contract.MaxLabelBytes.
func cutLabel(s string) string {
	cut, _ := adapter.Cut(s, contract.MaxLabelBytes)
	return cut
}
