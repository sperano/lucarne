package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

const (
	streamTimeout = 5 * time.Second
	testHeartbeat = 10 * time.Millisecond
	// secret stands for what an error of the source can carry: a token,
	// an internal host.
	secret = "token-0123456789 api.internal.example.com"
)

var (
	attempt   = streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerRunning)
	otherRun  = contract.RunKey{Cluster: streamtest.Cluster, Namespace: streamtest.Namespace, TaskUID: streamtest.RetryPodUID}
	documents = map[string]string{
		contract.SSETranscript: contract.DefEvent,
		contract.SSEAttempt:    contract.DefAttemptUpdate,
		contract.SSERun:        contract.DefRunUpdate,
		contract.SSEEnd:        contract.DefEnd,
		contract.SSEReset:      contract.DefReset,
		contract.SSELive:       contract.DefLive,
	}
)

// liveServer serves a hub reading src over HTTP.
func liveServer(t *testing.T, src stream.Source, options ...func(*stream.Config)) *httptest.Server {
	t.Helper()
	cfg := stream.Config{Source: src, Logger: slog.New(slog.DiscardHandler), PollInterval: time.Millisecond, RereadDelay: time.Millisecond}
	for _, o := range options {
		o(&cfg)
	}
	hub, err := stream.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(builtAssets(), Options{Runs: hub, Heartbeat: testHeartbeat, Logger: slog.New(slog.DiscardHandler)}))
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
	})
	return srv
}

// following is a source of a running run following the lines sent.
func following() (*streamtest.Source, chan<- string) {
	lines := make(chan string)
	src := streamtest.NewSource(contract.PhaseRunning, contract.RunnerUnknown)
	src.SetAttempts(attempt)
	src.AddReads(attempt.ID, streamtest.Follow(streamtest.Origin("http"), lines))
	return src, lines
}

func fetchSnapshot(t *testing.T, srv *httptest.Server) contract.Snapshot {
	t.Helper()
	res, err := http.Get(srv.URL + streamtest.Key.SnapshotPath())
	if err != nil {
		t.Fatal(err)
	}
	data := body(t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("snapshot: %d %s", res.StatusCode, data)
	}
	if err := contract.Validate(contract.DefSnapshot, []byte(data)); err != nil {
		t.Fatal(err)
	}
	var snap contract.Snapshot
	mustUnmarshal(t, data, &snap)
	return snap
}

// frame is one SSE message, or a comment.
type frame struct {
	event, id, data, comment string
}

// sse reads the frames of a stream.
type sse struct {
	t   *testing.T
	res *http.Response
	r   *bufio.Reader
}

// openStream opens the run's stream with cursor as after, and lastEventID
// as Last-Event-ID when it is not empty.
func openStream(t *testing.T, ctx context.Context, srv *httptest.Server, after, lastEventID string) *sse {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+streamtest.Key.EventsPath()+"?after="+after, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return &sse{t: t, res: res, r: bufio.NewReader(res.Body)}
}

// next is the next frame, checked against the schema; io.EOF at the end.
func (s *sse) next() (frame, error) {
	s.t.Helper()
	var f frame
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return f, err
		}
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, ":") {
			f.comment = line
			continue
		}
		field, value, _ := strings.Cut(line, ": ")
		switch field {
		case "":
			if f.comment != "" {
				return f, nil
			}
			if def, ok := documents[f.event]; !ok || contract.Validate(def, []byte(f.data)) != nil {
				s.t.Fatalf("frame %+v does not validate", f)
			}
			return f, nil
		case "event":
			f.event = value
		case "id":
			f.id = value
		case "data":
			f.data = value
		default:
			s.t.Fatalf("unexpected line %q", line)
		}
	}
}

// until reads frames up to the first of event, skipping comments.
func (s *sse) until(event string) []frame {
	s.t.Helper()
	var frames []frame
	for {
		f, err := s.next()
		if err != nil {
			s.t.Fatalf("stream ended before %s: %v", event, err)
		}
		if f.comment != "" {
			continue
		}
		frames = append(frames, f)
		if f.event == event {
			return frames
		}
	}
}

func mustUnmarshal(t *testing.T, data string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
}

func TestSnapshotEndpoint(t *testing.T) {
	t.Parallel()
	src, _ := following()
	snap := fetchSnapshot(t, liveServer(t, src))
	if snap.Run.Key != streamtest.Key || len(snap.Attempts) != 1 || snap.Stream.Epoch == "" {
		t.Errorf("snapshot %+v", snap)
	}
}

func TestStreamResumesFromTheSnapshotAndLastEventID(t *testing.T) {
	t.Parallel()
	src, lines := following()
	srv := liveServer(t, src)
	snap := fetchSnapshot(t, srv)
	cursor := fmt.Sprintf("%s:%d", snap.Stream.Epoch, snap.Stream.LastSeq)
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	first := openStream(t, ctx, srv, cursor, "")
	if ct := first.res.Header.Get("Content-Type"); first.res.StatusCode != http.StatusOK || ct != "text/event-stream" {
		t.Fatalf("stream: %d %s", first.res.StatusCode, ct)
	}
	// The replay (the attempt read since the snapshot) comes first.
	replayed := first.until(contract.SSELive)
	if live := replayed[len(replayed)-1]; live.id != "" || !strings.Contains(live.data, "lastSeq") {
		t.Fatalf("live %+v, want no id", live)
	}
	for _, f := range replayed[:len(replayed)-1] {
		if f.id == "" {
			t.Errorf("replayed %+v has no id", f)
		}
	}
	lines <- "one"
	got := first.until(contract.SSETranscript)
	last := got[len(got)-1]
	_ = first.res.Body.Close()
	lines <- "two"
	// The browser's reconnect: Last-Event-ID wins over after.
	second := openStream(t, ctx, srv, cursor, last.id)
	resumed := second.until(contract.SSETranscript)
	var ev contract.Event
	mustUnmarshal(t, resumed[len(resumed)-1].data, &ev)
	if ev.Raw == nil || ev.Raw.Text != "two" {
		t.Errorf("resumed with %+v, want line two", ev)
	}
	if resumed[0].event == contract.SSETranscript && strings.Contains(resumed[0].data, `"one"`) {
		t.Error("the update before Last-Event-ID was sent again")
	}
}

func TestStreamWithoutCursorResetsAndCloses(t *testing.T) {
	t.Parallel()
	src, _ := following()
	srv := liveServer(t, src)
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	s := openStream(t, ctx, srv, "", "")
	f, err := s.next()
	if err != nil || f.event != contract.SSEReset || f.id != "" || !strings.Contains(f.data, string(contract.ResetBadCursor)) {
		t.Fatalf("first frame %+v, %v; want a bad_cursor reset without id", f, err)
	}
	if _, err := s.next(); !errors.Is(err, io.EOF) {
		t.Errorf("after the reset: %v, want the stream closed", err)
	}
}

func TestFinishedStreamEndsThenAnswersNoContent(t *testing.T) {
	t.Parallel()
	src := streamtest.NewSource(contract.PhaseSucceeded, contract.RunnerUnknown)
	done := streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerTerminated)
	src.SetAttempts(done)
	src.AddReads(done.ID, streamtest.Deliver(streamtest.Origin("done"), streamtest.Lines(1, 2), kube.EndComplete, ""))
	srv := liveServer(t, src)
	snap := fetchSnapshot(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	s := openStream(t, ctx, srv, fmt.Sprintf("%s:%d", snap.Stream.Epoch, snap.Stream.LastSeq), "")
	end := s.until(contract.SSEEnd)
	if _, err := s.next(); !errors.Is(err, io.EOF) {
		t.Errorf("after the end: %v, want the stream closed", err)
	}
	again := openStream(t, ctx, srv, "", end[len(end)-1].id)
	if again.res.StatusCode != http.StatusNoContent {
		t.Errorf("reconnect after the end: %d, want 204", again.res.StatusCode)
	}
}

func TestIdleStreamGetsKeepalives(t *testing.T) {
	t.Parallel()
	src, _ := following()
	srv := liveServer(t, src)
	snap := fetchSnapshot(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), streamTimeout)
	defer cancel()
	s := openStream(t, ctx, srv, fmt.Sprintf("%s:%d", snap.Stream.Epoch, snap.Stream.LastSeq), "")
	for {
		f, err := s.next()
		if err != nil {
			t.Fatal(err)
		}
		if f.comment == strings.TrimSuffix(keepalive, "\n\n") {
			return
		}
	}
}

func TestDisconnectReleasesTheStream(t *testing.T) {
	t.Parallel()
	src, _ := following()
	srv := liveServer(t, src, func(c *stream.Config) { c.MaxRuns = 1 })
	snap := fetchSnapshot(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	s := openStream(t, ctx, srv, fmt.Sprintf("%s:%d", snap.Stream.Epoch, snap.Stream.LastSeq), "")
	s.until(contract.SSELive)
	if res := get(t, srv.Config.Handler, otherRun.SnapshotPath()); res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("another run while one is watched: %d, want 503", res.StatusCode)
	}
	cancel()
	deadline := time.Now().Add(streamTimeout)
	for {
		res := get(t, srv.Config.Handler, otherRun.SnapshotPath())
		if res.StatusCode == http.StatusNotFound {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run is still watched after its client left: %d", res.StatusCode)
		}
		time.Sleep(testHeartbeat)
	}
}

func TestHeadOfAStreamSendsNoEvent(t *testing.T) {
	t.Parallel()
	src, _ := following()
	srv := liveServer(t, src)
	res, err := http.Head(srv.URL + streamtest.Key.EventsPath())
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("HEAD: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func TestRunErrorsNameNoSecret(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		err    error
		key    contract.RunKey
		status int
		code   contract.ErrorCode
	}{
		"not found":   {nil, otherRun, http.StatusNotFound, contract.ErrorRunNotFound},
		"unavailable": {fmt.Errorf("%w: dial %s", kube.ErrUnavailable, secret), streamtest.Key, http.StatusServiceUnavailable, contract.ErrorUnavailable},
		"forbidden":   {fmt.Errorf("%w: %s", kube.ErrForbidden, secret), streamtest.Key, http.StatusServiceUnavailable, contract.ErrorUnavailable},
		"other":       {errors.New(secret), streamtest.Key, http.StatusInternalServerError, contract.ErrorInternal},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := streamtest.NewSource(contract.PhaseRunning, contract.RunnerUnknown)
			src.SetRunError(tt.err)
			srv := liveServer(t, src)
			for _, path := range []string{tt.key.SnapshotPath(), tt.key.EventsPath()} {
				res := get(t, srv.Config.Handler, path)
				data := body(t, res)
				if res.StatusCode != tt.status || !strings.Contains(data, `"code":"`+string(tt.code)+`"`) {
					t.Errorf("GET %s: %d %s, want %d %s", path, res.StatusCode, data, tt.status, tt.code)
				}
				if err := contract.Validate(contract.DefErrorResponse, []byte(data)); err != nil {
					t.Error(err)
				}
				if strings.Contains(data, "token") || strings.Contains(data, "internal.example") {
					t.Errorf("GET %s answered the source's error: %s", path, data)
				}
			}
		})
	}
}
