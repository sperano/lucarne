package kubetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// LineInterval is the time between the stamps of two lines of a log.
const LineInterval = 10 * time.Millisecond

// ErrBroken is how a broken log stream ends.
var ErrBroken = errors.New("kubetest: log stream broken")

// LogOpen is one log stream a reader opened.
type LogOpen struct {
	Namespace string
	Pod       string
	Options   corev1.PodLogOptions
}

// Logs serves the agent containers' logs as the kubelet does, through
// the pods/log subresource: stamped when asked, from a time, followed until
// the container terminates. It implements kube.LogStreamer.
type Logs struct {
	mu    sync.Mutex
	logs  map[string]*podLog
	opens []LogOpen
}

func newLogs() *Logs { return &Logs{logs: map[string]*podLog{}} }

func (l *Logs) add(namespace, pod string) *podLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	log := &podLog{changed: make(chan struct{})}
	l.logs[namespace+"/"+pod] = log
	return log
}

// Opens are the streams opened so far, in order.
func (l *Logs) Opens() []LogOpen {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogOpen(nil), l.opens...)
}

// Stream opens the log of pod's agent container as opts says. A pod with
// no log is not found; a previous instance has none.
func (l *Logs) Stream(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	l.mu.Lock()
	l.opens = append(l.opens, LogOpen{Namespace: namespace, Pod: pod, Options: *opts})
	log := l.logs[namespace+"/"+pod]
	l.mu.Unlock()
	if log == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, pod)
	}
	if opts.Previous {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("previous terminated container %q in pod %q not found", opts.Container, pod))
	}
	return log.open(ctx, opts), nil
}

// podLog is one container's log.
type podLog struct {
	mu    sync.Mutex
	lines []stampedLine
	// start is the first line the log serves, after a rotation.
	start      int
	terminated bool
	// breaks counts the breaks; a stream opened before the last one ends.
	breaks int
	// changed is closed, and replaced, on every change.
	changed chan struct{}
}

type stampedLine struct {
	at   time.Time
	text string
}

func (p *podLog) write(lines []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, text := range lines {
		at := Epoch.Add(time.Duration(len(p.lines)+1) * LineInterval)
		p.lines = append(p.lines, stampedLine{at: at, text: text})
	}
	p.notify()
}

func (p *podLog) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.lines)
}

func (p *podLog) lastStamp() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.lines) == 0 {
		return Epoch
	}
	return p.lines[len(p.lines)-1].at
}

func (p *podLog) breakStreams() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.breaks++
	p.notify()
}

func (p *podLog) rotate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.start = len(p.lines)
}

func (p *podLog) terminate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.terminated = true
	p.notify()
}

// notify wakes the streams waiting for a change. p.mu must be held.
func (p *podLog) notify() {
	close(p.changed)
	p.changed = make(chan struct{})
}

func (p *podLog) open(ctx context.Context, opts *corev1.PodLogOptions) io.ReadCloser {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := p.start
	if opts.SinceTime != nil {
		for next < len(p.lines) && p.lines[next].at.Before(opts.SinceTime.Time) {
			next++
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return &logStream{log: p, ctx: ctx, cancel: cancel, next: next, breaks: p.breaks, follow: opts.Follow, stamps: opts.Timestamps}
}

// logStream reads a podLog from its next line.
type logStream struct {
	log            *podLog
	ctx            context.Context
	cancel         context.CancelFunc
	next, breaks   int
	follow, stamps bool
	buf            bytes.Buffer
}

func (s *logStream) Read(b []byte) (int, error) {
	for s.buf.Len() == 0 {
		if err := s.fill(); err != nil {
			return 0, err
		}
	}
	return s.buf.Read(b)
}

// fill buffers the next line, waiting for one when following.
func (s *logStream) fill() error {
	for {
		p := s.log
		p.mu.Lock()
		switch {
		case s.ctx.Err() != nil:
			p.mu.Unlock()
			return s.ctx.Err()
		case p.breaks != s.breaks:
			p.mu.Unlock()
			return ErrBroken
		case s.next < len(p.lines):
			s.render(p.lines[s.next])
			s.next++
			p.mu.Unlock()
			return nil
		case !s.follow || p.terminated:
			p.mu.Unlock()
			return io.EOF
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-s.ctx.Done():
		}
	}
}

func (s *logStream) render(l stampedLine) {
	if s.stamps {
		s.buf.WriteString(l.at.UTC().Format(time.RFC3339Nano))
		s.buf.WriteByte(' ')
	}
	s.buf.WriteString(l.text)
	s.buf.WriteByte('\n')
}

// Close ends the stream; a Read waiting for a line returns.
func (s *logStream) Close() error {
	s.cancel()
	return nil
}
