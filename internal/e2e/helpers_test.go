// Package e2e holds the integration tests that read the fake cluster of
// internal/e2e/scenario through the real Kubernetes source, stream hub and
// HTTP server, and talk to the server over HTTP only.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/server"
	"github.com/sperano/lucarne/internal/stream"
)

const (
	// pollInterval and rereadDelay are short, so that the tests see a
	// change soon after they make it.
	pollInterval = 5 * time.Millisecond
	rereadDelay  = 5 * time.Millisecond
	// heartbeat is the server's keepalive interval.
	heartbeat = 100 * time.Millisecond
	// settleTimeout bounds the wait of a first snapshot for the logs of a
	// finished run; the long session needs seconds under the race detector.
	settleTimeout = 60 * time.Second
	// waitTimeout is the deadline of everything a test waits for.
	waitTimeout = 90 * time.Second
	// retryEvery is how often a test looks again at what it waits for.
	retryEvery = 5 * time.Millisecond
	// manyRuns lets a hub follow every run of the world at once.
	manyRuns = 64
)

// indexHTML stands for the built UI, which these tests do not exercise.
var indexHTML = fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>lucarne</title>")}}

var (
	worldOnce sync.Once
	theWorld  *scenario.World
	worldErr  error
)

// sharedWorld is the world every test reads: building it takes seconds,
// and a test that changes a run uses a live run of its own.
func sharedWorld(t *testing.T) *scenario.World {
	t.Helper()
	worldOnce.Do(func() { theWorld, worldErr = scenario.New() })
	if worldErr != nil {
		t.Fatalf("build the world: %v", worldErr)
	}
	return theWorld
}

// stack is the world read through a hub and served over HTTP.
type stack struct {
	t      *testing.T
	world  *scenario.World
	hub    *stream.Hub
	srv    *httptest.Server
	client *http.Client
	logs   *logBuffer
	rec    *recorder
	texts  *textSet
}

// newStack serves the shared world through a hub configured by mutate. At
// the end of the test it checks what every test must keep true: the logs
// hold no transcript, and the cluster was only read.
func newStack(t *testing.T, mutate ...func(*stream.Config)) *stack {
	t.Helper()
	return newStackWith(t, false, mutate...)
}

// newStackWith is newStack; smallBuffers gives the server's connections a
// tiny send buffer, so that a client that does not read blocks the
// server's writes after a few kilobytes.
func newStackWith(t *testing.T, smallBuffers bool, mutate ...func(*stream.Config)) *stack {
	t.Helper()
	w := sharedWorld(t)
	src, err := w.Source()
	if err != nil {
		t.Fatal(err)
	}
	logs := &logBuffer{}
	logger := logs.logger()
	cfg := stream.Config{
		Source: src, Logger: logger, PollInterval: pollInterval, RereadDelay: rereadDelay,
		SettleTimeout: settleTimeout, MaxRuns: manyRuns,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	hub, err := stream.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(server.New(indexHTML, server.Options{Runs: hub, Catalog: src, Logger: logger, Heartbeat: heartbeat}))
	if smallBuffers {
		srv.Listener = smallSendBuffers{srv.Listener}
	}
	srv.Start()
	rec := &recorder{}
	st := &stack{
		t: t, world: w, hub: hub, srv: srv, logs: logs, rec: rec, texts: &textSet{},
		client: &http.Client{Transport: &recordingTransport{base: http.DefaultTransport, rec: rec}},
	}
	t.Cleanup(st.checkHygiene)
	t.Cleanup(srv.Close)
	t.Cleanup(hub.Close)
	return st
}

// smallSendBuffers is a listener whose connections have a tiny send buffer.
type smallSendBuffers struct{ net.Listener }

// tinyBuffer is the buffer size asked for; the kernel rounds it up.
const tinyBuffer = 4096

func (l smallSendBuffers) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if tcp, ok := conn.(*net.TCPConn); ok && err == nil {
		_ = tcp.SetWriteBuffer(tinyBuffer)
	}
	return conn, err
}

// slowClient is a client with a tiny receive buffer, whose responses are
// recorded as the stack's others are.
func (s *stack) slowClient() *http.Client {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if tcp, ok := conn.(*net.TCPConn); ok && err == nil {
				_ = tcp.SetReadBuffer(tinyBuffer)
			}
			return conn, err
		},
	}
	return &http.Client{Transport: &recordingTransport{base: transport, rec: s.rec}}
}

// with is the stack as seen by a subtest t.
func (s *stack) with(t *testing.T) *stack {
	c := *s
	c.t = t
	return &c
}

// response is a whole HTTP response.
type response struct {
	URL    string
	Status int
	Header http.Header
	Body   []byte
}

// get requests path, following redirects, with extra header fields given
// as name, value pairs.
func (s *stack) get(path string, header ...string) response {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.srv.URL+path, nil)
	if err != nil {
		s.t.Fatalf("request %s: %v", path, err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatalf("read GET %s: %v", path, err)
	}
	return response{URL: path, Status: res.StatusCode, Header: res.Header, Body: body}
}

// apiError checks that r is the API error status and code say, a valid
// ErrorResponse, and returns it.
func (s *stack) apiError(r response, status int, code contract.ErrorCode) contract.ErrorBody {
	s.t.Helper()
	if r.Status != status {
		s.t.Fatalf("GET %s: status %d, want %d: %s", r.URL, r.Status, status, r.Body)
	}
	if err := contract.Validate(contract.DefErrorResponse, r.Body); err != nil {
		s.t.Fatalf("GET %s: %v\n%s", r.URL, err, r.Body)
	}
	var e contract.ErrorResponse
	mustUnmarshal(s.t, r.Body, &e)
	if e.Error.Code != code {
		s.t.Errorf("GET %s: error code %q, want %q", r.URL, e.Error.Code, code)
	}
	return e.Error
}

// snapshot is the run's snapshot, which must be a valid 200.
func (s *stack) snapshot(key contract.RunKey) contract.Snapshot {
	s.t.Helper()
	snap, _ := s.snapshotResponse(key)
	return snap
}

// snapshotResponse is the run's snapshot and its response.
func (s *stack) snapshotResponse(key contract.RunKey) (contract.Snapshot, response) {
	s.t.Helper()
	return s.decodeSnapshot(key, false)
}

// sampledEvents is how many events at each end of a huge snapshot are
// validated.
const sampledEvents = 200

// hugeSnapshot is a snapshot of thousands of events: validating all of
// them takes tens of seconds under the race detector, so the document is
// validated with its first and last sampledEvents events only. The same
// schema checks every event of the other snapshots.
func (s *stack) hugeSnapshot(key contract.RunKey) (contract.Snapshot, response) {
	s.t.Helper()
	return s.decodeSnapshot(key, true)
}

func (s *stack) decodeSnapshot(key contract.RunKey, sampled bool) (contract.Snapshot, response) {
	s.t.Helper()
	r := s.get(key.SnapshotPath())
	if r.Status != http.StatusOK {
		s.t.Fatalf("snapshot of %s: status %d: %s", key.LinkPath(), r.Status, truncateForLog(r.Body))
	}
	checkJSONHeaders(s.t, r)
	document := r.Body
	if sampled {
		document = sampleEvents(s.t, document)
	}
	if err := contract.Validate(contract.DefSnapshot, document); err != nil {
		s.t.Fatalf("snapshot of %s: %v", key.LinkPath(), err)
	}
	var snap contract.Snapshot
	mustUnmarshal(s.t, r.Body, &snap)
	s.texts.add(snap.Events)
	return snap, r
}

// sampleEvents is a snapshot document with only the ends of its events.
func sampleEvents(t *testing.T, document []byte) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	mustUnmarshal(t, document, &fields)
	var events []json.RawMessage
	mustUnmarshal(t, fields["events"], &events)
	if len(events) > 2*sampledEvents {
		events = append(events[:sampledEvents:sampledEvents], events[len(events)-sampledEvents:]...)
	}
	sampled, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	fields["events"] = sampled
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// checkJSONHeaders checks the headers of an API response: JSON, not
// cached, not sniffable, under a same-origin content security policy.
func checkJSONHeaders(t *testing.T, r response) {
	t.Helper()
	want := map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	for name, value := range want {
		if got := r.Header.Get(name); got != value {
			t.Errorf("GET %s: %s = %q, want %q", r.URL, name, got, value)
		}
	}
	csp := r.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("GET %s: Content-Security-Policy = %q, want a same-origin policy", r.URL, csp)
	}
}

// cursor is the stream position a snapshot reflects, as an after
// parameter.
func cursor(snap contract.Snapshot) string {
	return stream.Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq}.String()
}

// eventually calls cond every retryEvery until it holds, failing the test
// after waitTimeout.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up waiting for %s", what)
		}
		time.Sleep(retryEvery)
	}
}

func mustUnmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode %.200s: %v", data, err)
	}
}

// truncateForLog shortens a body for a failure message.
func truncateForLog(b []byte) string {
	const limit = 400
	if len(b) > limit {
		return string(b[:limit]) + "..."
	}
	return string(b)
}

// recorder keeps every byte the server sent, to look for what it must
// never send.
type recorder struct {
	mu      sync.Mutex
	entries []*recorded
}

type recorded struct {
	url  string
	data bytes.Buffer
}

func (r *recorder) start(url string) *recorded {
	e := &recorded{url: url}
	r.mu.Lock()
	r.entries = append(r.entries, e)
	r.mu.Unlock()
	return e
}

// find returns the URLs of the responses that held fragment.
func (r *recorder) find(fragment string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var urls []string
	for _, e := range r.entries {
		if bytes.Contains(e.data.Bytes(), []byte(fragment)) {
			urls = append(urls, e.url)
		}
	}
	return urls
}

// recordingTransport copies what every response body delivers into rec.
type recordingTransport struct {
	base http.RoundTripper
	rec  *recorder
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err != nil {
		return res, err
	}
	res.Body = &recordedBody{rc: res.Body, rec: t.rec, entry: t.rec.start(req.URL.RequestURI())}
	return res, nil
}

type recordedBody struct {
	rc    io.ReadCloser
	rec   *recorder
	entry *recorded
}

func (b *recordedBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.rec.mu.Lock()
		b.entry.data.Write(p[:n])
		b.rec.mu.Unlock()
	}
	return n, err
}

func (b *recordedBody) Close() error { return b.rc.Close() }

// assertNeverSent fails when a response held one of the fragments.
func (s *stack) assertNeverSent(fragments ...string) {
	s.t.Helper()
	for _, f := range fragments {
		for _, url := range s.rec.find(f) {
			s.t.Errorf("response to %s holds %q", url, f)
		}
	}
}
