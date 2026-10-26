// Package streamtest holds what the tests of the stream and of the server
// share: a scripted source of runs and logs, and fabricated identities.
package streamtest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
)

// Fabricated identities: the repository is public.
const (
	Cluster     = "example"
	Namespace   = "agents"
	TaskName    = "example-claude-1"
	TaskUID     = "3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"
	PodName     = "example-claude-1-abcde"
	PodUID      = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f"
	RetryPodUID = "8d0e1f2a-3b4c-4d5e-8f70-8b9c0d1e2f3a"
)

// Key is the fabricated run's key.
var Key = contract.RunKey{Cluster: Cluster, Namespace: Namespace, TaskUID: TaskUID}

// BaseTime is when the fabricated run's lines start.
var BaseTime = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

// Read is one scripted ReadLog: it delivers to sink, then returns.
type Read func(ctx context.Context, sink kube.LogSink) (kube.ReadResult, error)

// Source is a scripted run source. Each read of an attempt takes the next
// of its scripted reads; once they are used up, a read waits for its
// context, as a follow of a log nobody writes to. It is safe for
// concurrent use.
type Source struct {
	mu          sync.Mutex
	run         contract.Run
	runErr      error
	refreshErr  error
	attemptsErr error
	attempts    []contract.Attempt
	reads       map[string][]Read
	opens       map[string][]kube.ReadOptions
	runCalls    int
}

// NewSource is a source of the fabricated run, in phase, of runner.
func NewSource(phase contract.RunPhase, runner contract.RunnerKind) *Source {
	return &Source{
		run:   contract.Run{Key: Key, TaskName: TaskName, Runner: runner, Phase: phase},
		reads: map[string][]Read{},
		opens: map[string][]kube.ReadOptions{},
	}
}

// SetPhase changes the run's phase.
func (s *Source) SetPhase(phase contract.RunPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Phase = phase
}

// SetRunError makes Run fail with err; nil makes it succeed again.
func (s *Source) SetRunError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runErr = err
}

// SetRefreshError makes Refresh fail with err, kube.ErrRunDeleted for a
// deleted Task.
func (s *Source) SetRefreshError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshErr = err
}

// SetAttemptsError makes Attempts fail with err.
func (s *Source) SetAttemptsError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attemptsErr = err
}

// SetAttempts sets the attempts listed.
func (s *Source) SetAttempts(attempts ...contract.Attempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts = slices.Clone(attempts)
}

// AddReads scripts the next reads of the attempt with id.
func (s *Source) AddReads(id string, reads ...Read) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads[id] = append(s.reads[id], reads...)
}

// Opens are the options of every read of the attempt with id so far.
func (s *Source) Opens(id string) []kube.ReadOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.opens[id])
}

// RunCalls is how many times Run was called.
func (s *Source) RunCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runCalls
}

// Run implements stream.Source: the fabricated run, by its key only.
func (s *Source) Run(_ context.Context, key contract.RunKey) (kube.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runCalls++
	if s.runErr != nil {
		return kube.Run{}, s.runErr
	}
	if key != Key {
		return kube.Run{}, fmt.Errorf("%w: %s", kube.ErrRunNotFound, key.LinkPath())
	}
	return kube.Run{Run: s.run}, nil
}

// Refresh implements stream.Source.
func (s *Source) Refresh(context.Context, kube.Run) (kube.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshErr != nil {
		return kube.Run{}, s.refreshErr
	}
	return kube.Run{Run: s.run}, nil
}

// Attempts implements stream.Source.
func (s *Source) Attempts(context.Context, kube.Run) ([]contract.Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attemptsErr != nil {
		return nil, s.attemptsErr
	}
	return slices.Clone(s.attempts), nil
}

// ReadLog implements stream.Source with the attempt's next scripted read.
func (s *Source) ReadLog(ctx context.Context, _ kube.Run, key contract.AttemptKey, opts kube.ReadOptions, sink kube.LogSink) (kube.ReadResult, error) {
	id := key.ID()
	s.mu.Lock()
	s.opens[id] = append(s.opens[id], opts)
	var read Read
	if reads := s.reads[id]; len(reads) > 0 {
		read, s.reads[id] = reads[0], reads[1:]
	}
	s.mu.Unlock()
	if read == nil {
		<-ctx.Done()
		return kube.ReadResult{End: kube.EndCanceled, Reason: kube.ReasonCanceled}, ctx.Err()
	}
	return read(ctx, sink)
}

// Attempt is attempt restart of the agent container of the pod with uid,
// in state, pending as the source lists a log not read yet.
func Attempt(podUID string, restart int64, state contract.ContainerStateKind) contract.Attempt {
	key := contract.AttemptKey{PodUID: podUID, Container: kube.AgentContainer, Restart: restart}
	return contract.Attempt{
		ID: key.ID(), Key: key, PodName: PodName,
		Runner:    contract.Runner{Kind: contract.RunnerUnknown},
		Container: contract.ContainerState{State: state},
		Coverage:  contract.Coverage{State: contract.CoveragePending},
	}
}

// Line is line number of a log, written at BaseTime plus number
// milliseconds.
func Line(number int64, text string) adapter.Line {
	at := contract.Timestamp{Time: BaseTime.Add(time.Duration(number) * time.Millisecond)}
	return adapter.Line{Number: number, Text: []byte(text), Bytes: int64(len(text)), LogTime: &at}
}

// Lines are count lines numbered from, each a distinct JSON object.
func Lines(from, count int64) []adapter.Line {
	lines := make([]adapter.Line, 0, count)
	for n := from; n < from+count; n++ {
		lines = append(lines, Line(n, fmt.Sprintf(`{"n":%d}`, n)))
	}
	return lines
}

// Origin is a fabricated log origin.
func Origin(name string) string { return contract.LogOrigin([]byte(name)) }

// Deliver is a read of lines from origin that ends as end says.
func Deliver(origin string, lines []adapter.Line, end kube.ReadEnd, reason string) Read {
	return func(_ context.Context, sink kube.LogSink) (kube.ReadResult, error) {
		res := kube.ReadResult{Origin: origin, End: end, Reason: reason}
		for _, l := range lines {
			if err := sink.Line(origin, l); err != nil {
				return res, err
			}
			res.Lines++
		}
		return res, nil
	}
}

// Follow is a read of the lines sent on lines from origin, numbered from
// 1, until lines is closed (the container terminated) or ctx is done.
func Follow(origin string, lines <-chan string) Read {
	return func(ctx context.Context, sink kube.LogSink) (kube.ReadResult, error) {
		res := kube.ReadResult{Origin: origin}
		for {
			select {
			case <-ctx.Done():
				res.End, res.Reason = kube.EndCanceled, kube.ReasonCanceled
				return res, ctx.Err()
			case text, ok := <-lines:
				if !ok {
					res.End = kube.EndComplete
					return res, nil
				}
				res.Lines++
				if err := sink.Line(origin, Line(res.Lines, text)); err != nil {
					return res, err
				}
			}
		}
	}
}
