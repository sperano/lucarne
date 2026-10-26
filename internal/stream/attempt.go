package stream

import (
	"context"
	"reflect"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// attempt is one attempt of a run: as the source lists it, and how much of
// its log was read. Its fields are the run's, under its lock, but for sink,
// which its reader owns while it runs.
type attempt struct {
	key contract.AttemptKey
	// listed is the attempt as the source last listed it.
	listed contract.Attempt
	// published is the attempt as last sent.
	published contract.Attempt
	// settles is an attempt the first snapshot waits for.
	settles bool

	// reading is a read in progress; read is one started at least once.
	reading, read bool
	// end and reason say how the last read ended.
	end    kube.ReadEnd
	reason string
	// lines, gaps and truncated count what was delivered, over every read.
	lines, gaps, truncated int64

	// halted is a reader told to stop: what it still reads is dropped.
	halted bool
	cancel context.CancelFunc
	sink   *logSink
}

func newAttempt(listed contract.Attempt) *attempt {
	return &attempt{key: listed.Key, listed: listed}
}

// readable is an attempt whose log can be read: the source lists it as
// pending until it is.
func (a *attempt) readable() bool {
	return a.listed.Coverage.State == contract.CoveragePending
}

// current is the attempt's state now: the listed one until a read starts,
// then the container as listed and the coverage of the reads.
func (a *attempt) current() contract.Attempt {
	current := a.listed
	if !a.read {
		return current
	}
	result := kube.ReadResult{Lines: a.lines, Gaps: a.gaps, TruncatedLines: a.truncated, End: a.end, Reason: a.reason}
	switch {
	case a.reading && current.Container.State == contract.ContainerRunning:
		result.End, result.Reason = kube.EndCurrent, ""
	case a.reading:
		// Waiting for the container to start, or reading a log that is
		// complete: not read yet.
		result.End, result.Reason = kube.EndPending, ""
		if a.listed.Coverage.State == contract.CoveragePending {
			result.Reason = a.listed.Coverage.Reason
		}
	}
	current.Coverage = result.Coverage()
	return current
}

// final is an attempt whose log will not change: read to its end, or gone.
func (a *attempt) final() bool {
	if a.reading {
		return false
	}
	state := a.current().Coverage.State
	return state == contract.CoverageEnded || state == contract.CoverageExpired
}

// finalEnd is a read end after which the log is not read again.
func finalEnd(end kube.ReadEnd) bool {
	return end == kube.EndComplete || end == kube.EndLimit || end == kube.EndExpired
}

// equal compares two states of the contract, pointers by what they hold.
func equal[T contract.Attempt | contract.Run](a, b T) bool {
	return reflect.DeepEqual(a, b)
}
