package kube

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// maxFailures is how many tries in a row may fail to check the instance or
// open its log stream before the log is reported unavailable.
const maxFailures = 3

// maxQuietBreaks is how many log streams in a row may open and then break
// with no new line before the log is reported unavailable. A stream that
// opens resets the count of failures, so this bounds a log whose stream
// always breaks. It is above maxFailures: the API answered each of them,
// and a follow breaks while the agent writes nothing.
const maxQuietBreaks = 5

// errPodGone is a pod that is no longer one of the run's.
var errPodGone = errors.New("pod gone")

// LogSink receives what a log read finds, in order. An error it returns
// stops the read and is returned by ReadLog.
type LogSink interface {
	// Line is the next line of the log read from origin.
	Line(origin string, line adapter.Line) error
	// Gap says that lines are missing before the line numbered next. A
	// read reports one only once it has an origin, after a first line.
	Gap(origin string, next int64, gap contract.Gap) error
}

// ReadOptions says how to read an attempt's log.
type ReadOptions struct {
	// Follow keeps reading while the instance writes, until it terminates
	// or the context is done; a container not started yet is waited for.
	// Without it, the read is a snapshot of the log as it is.
	Follow bool
	// KnownOrigin is the origin of an earlier read of this attempt, empty
	// for none. A first line with another origin means the log's start
	// is gone, a source_truncated gap.
	KnownOrigin string
}

// ReadLog reads the log of run's attempt key into sink, bounded by
// contract.MaxLineBytes and contract.MaxAttemptLogBytes. Before each
// stream it opens, it checks the Task by UID, its Job, the pod by UID and
// the instance; an instance that is gone or cannot be read ends the read
// with the result saying so. The error is ErrAttemptNotFound for a key that
// names no instance of the run, ErrRunNotFound for a run outside the
// allowlist, the sink's, or the context's (End is then EndCanceled).
func (s *Source) ReadLog(ctx context.Context, run Run, key contract.AttemptKey, opts ReadOptions, sink LogSink) (ReadResult, error) {
	if err := s.readable(run.Key); err != nil {
		return ReadResult{}, err
	}
	if err := key.Validate(); err != nil || key.Container != AgentContainer {
		return ReadResult{}, fmt.Errorf("%w: %s", ErrAttemptNotFound, key.ID())
	}
	rd := &reader{s: s, run: run, key: key, opts: opts, sink: sink}
	err := rd.read(ctx)
	if ctx.Err() != nil {
		rd.res.End, rd.res.Reason = EndCanceled, ReasonCanceled
		return rd.res, ctx.Err()
	}
	return rd.res, err
}

// reader is one ReadLog call.
type reader struct {
	s        *Source
	run      Run
	key      contract.AttemptKey
	opts     ReadOptions
	sink     LogSink
	res      ReadResult
	tail     tail
	failures int
	// breaks counts the quiet breaks in a row, brokenAt the lines read
	// at the last break.
	breaks   int
	brokenAt int64
}

// instance is the container instance an attempt names, as last checked.
type instance struct {
	pod      string
	restarts int32
	previous bool
	started  bool
	// terminated is an instance whose log is complete.
	terminated bool
}

// read opens streams until the log is read, gone or unreadable.
func (rd *reader) read(ctx context.Context) error {
	for {
		inst, done, err := rd.locate(ctx)
		if done || err != nil {
			return err
		}
		switch {
		case inst == nil:
			// A check failed: try again.
		case inst.started:
			if done, err := rd.stream(ctx, *inst); done || err != nil {
				return err
			}
		case !rd.opts.Follow:
			return rd.end(ctx, EndPending, ReasonNotStarted)
		}
		if err := rd.s.wait(ctx); err != nil {
			return err
		}
	}
}

// fail counts a failed try; after maxFailures in a row the log is
// reported unavailable.
func (rd *reader) fail(ctx context.Context, err error) (bool, error) {
	if errors.Is(err, ErrForbidden) {
		return true, rd.end(ctx, EndUnavailable, ReasonForbidden)
	}
	if rd.failures++; rd.failures >= maxFailures {
		return true, rd.end(ctx, EndUnavailable, ReasonSourceUnavailable)
	}
	return false, nil
}

// end records why the read stopped. An instance that went away after lines
// were read leaves the rest of its log missing: an attempt_expired gap.
func (rd *reader) end(ctx context.Context, end ReadEnd, reason string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	rd.res.End, rd.res.Reason = end, reason
	if end == EndExpired && rd.res.Origin != "" {
		return rd.gap(contract.Gap{Reason: contract.GapAttemptExpired, Detail: reason})
	}
	return nil
}

func (rd *reader) gap(gap contract.Gap) error {
	rd.res.Gaps++
	return rd.sink.Gap(rd.res.Origin, rd.res.Lines+1, gap)
}

// locate checks the whole chain to the instance, and ends the read when it
// is gone or cannot be read. A nil instance with the read not over is a
// check that failed, to try again.
func (rd *reader) locate(ctx context.Context) (*instance, bool, error) {
	pod, err := rd.s.ownedPod(ctx, rd.run, rd.key.PodUID)
	switch {
	case ctx.Err() != nil:
		return nil, true, ctx.Err()
	case errors.Is(err, ErrRunDeleted):
		return nil, true, rd.end(ctx, EndExpired, ReasonRunDeleted)
	case errors.Is(err, errPodGone):
		return nil, true, rd.end(ctx, EndExpired, ReasonPodDeleted)
	case err != nil:
		done, err := rd.fail(ctx, err)
		return nil, done, err
	}
	inst, end, reason, err := rd.instance(pod)
	if err != nil {
		return nil, true, err
	}
	if end != "" {
		return nil, true, rd.end(ctx, end, reason)
	}
	return &inst, false, nil
}

// instance finds the instance the attempt key names in pod: the current
// one, the previous one while Kubernetes keeps its log, or none. While the
// container backs off, the instance that exited is the only one with a log.
func (rd *reader) instance(pod *corev1.Pod) (instance, ReadEnd, string, error) {
	notFound := fmt.Errorf("%w: %s", ErrAttemptNotFound, rd.key.ID())
	if !hasAgentContainer(pod) {
		return instance{}, "", "", notFound
	}
	inst := instance{pod: pod.Name}
	status := agentStatus(pod)
	if status != nil {
		inst.restarts = status.RestartCount
	}
	switch restart := rd.key.Restart; {
	case restart > int64(inst.restarts):
		return instance{}, "", "", notFound
	case restart == int64(inst.restarts) && status != nil && backingOff(status):
		// Previous names this instance before and after the kubelet
		// restarts the container, even while the pod's status lags.
		inst.previous, inst.started, inst.terminated = true, true, true
	case restart == int64(inst.restarts):
		if status != nil {
			inst.started = status.State.Running != nil || status.State.Terminated != nil
			inst.terminated = status.State.Terminated != nil
		}
		if !inst.started && podFinished(pod) {
			return instance{}, EndExpired, ReasonNeverStarted, nil
		}
	case restart == int64(inst.restarts)-1 && status != nil && status.LastTerminationState.Terminated != nil && !backingOff(status):
		inst.previous, inst.started, inst.terminated = true, true, true
	default:
		return instance{}, EndExpired, ReasonOlderRestart, nil
	}
	return inst, "", "", nil
}

// stream reads one log stream of inst, once its pod is checked again after
// it opened. It reports whether the read is over.
func (rd *reader) stream(ctx context.Context, inst instance) (bool, error) {
	if rd.tail.untimed {
		return true, rd.interrupted(ctx)
	}
	follow := rd.opts.Follow && !inst.terminated
	opts := &corev1.PodLogOptions{Container: AgentContainer, Previous: inst.previous, Follow: follow, Timestamps: true}
	if !rd.tail.empty() {
		opts.SinceTime = &metav1.Time{Time: rd.tail.since}
	}
	body, err := rd.s.clients.Logs.Stream(ctx, rd.run.Key.Namespace, inst.pod, opts)
	if err != nil {
		return rd.fail(ctx, classify(ctx, err, "open log"))
	}
	// A read-only stream has nothing to flush: its Close error is moot.
	defer func() { _ = body.Close() }()
	same, err := rd.s.samePod(ctx, rd.run.Key.Namespace, inst.pod, rd.key.PodUID)
	if err != nil {
		return rd.fail(ctx, err)
	}
	if same == nil || restartsOf(same) != inst.restarts {
		// The pod changed between the check and the open: check again.
		return false, nil
	}
	rd.failures = 0
	return rd.consume(ctx, newLineReader(body), inst, follow)
}

// interrupted ends a read that cannot resume: lines after the last one read
// may be missing.
func (rd *reader) interrupted(ctx context.Context) error {
	if err := rd.gap(contract.Gap{Reason: contract.GapReadInterrupted, Detail: ReasonNoResume}); err != nil {
		return err
	}
	return rd.end(ctx, EndUnavailable, ReasonNoResume)
}

func restartsOf(pod *corev1.Pod) int32 {
	if status := agentStatus(pod); status != nil {
		return status.RestartCount
	}
	return 0
}
