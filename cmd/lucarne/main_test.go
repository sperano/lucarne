package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/stream"
)

const runTestTimeout = 5 * time.Second

func TestRunServesUntilCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, listener, options{logger: slog.New(slog.DiscardHandler)}) }()

	res, err := http.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("healthz status %d", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after cancel, want nil", err)
		}
	case <-time.After(runTestTimeout):
		t.Fatal("run did not return after cancel")
	}
}

func TestRunDrainsBeforeShutdown(t *testing.T) {
	const drainDelay = 300 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, listener, options{drainDelay: drainDelay, logger: slog.New(slog.DiscardHandler)})
	}()
	if status := statusOf(t, base+"/readyz"); status != http.StatusOK {
		t.Fatalf("readyz %d before the signal", status)
	}

	cancel()
	deadline := time.Now().Add(drainDelay)
	for statusOf(t, base+"/readyz") != http.StatusServiceUnavailable {
		if time.Now().After(deadline) {
			t.Fatal("readyz never answered 503 while draining")
		}
	}
	if status := statusOf(t, base+"/healthz"); status != http.StatusOK {
		t.Errorf("healthz %d while draining", status)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after draining, want nil", err)
		}
	case <-time.After(runTestTimeout):
		t.Fatal("run did not return after draining")
	}
}

// statusOf is the status of GET url.
func statusOf(t *testing.T, url string) int {
	t.Helper()
	res := get(t, context.Background(), url)
	_ = res.Body.Close()
	return res.StatusCode
}

const (
	// followPoll is how often the hub of the shutdown test reads the fake
	// cluster.
	followPoll = 5 * time.Millisecond
	// liveMarker is the SSE event that ends a stream's replay.
	liveMarker = "event: live"
	// clientTimeout bounds the test's HTTP requests.
	clientTimeout = time.Minute
	// endMarker would end a stream whose run finished: a stopping server
	// sends none, since the run goes on and the client reconnects.
	endMarker = "event: end"
)

// TestRunShutdownEndsOpenStreams starts run over a hub that follows a live
// run, opens its SSE stream, stops the server and checks that the open
// stream does not hold the shutdown: the stream ends, with no end event
// (the run did not end), and run returns nil.
func TestRunShutdownEndsOpenStreams(t *testing.T) {
	live, hub := liveHub(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, listener, options{runs: hub, logger: slog.New(slog.DiscardHandler)}) }()

	// The client has a context of its own: cancelling the server's must
	// not be what ends its stream.
	clientCtx, stopClient := context.WithTimeout(context.Background(), clientTimeout)
	defer stopClient()
	events := openStream(t, clientCtx, "http://"+listener.Addr().String(), live.Key)
	defer func() { _ = events.Body.Close() }()
	reader := bufio.NewReader(events.Body)
	awaitLine(t, reader, liveMarker)
	live.Pod.Write(scenario.ClaudeText(1, "written before the shutdown"))
	awaitLine(t, reader, "event: transcript")

	cancel()
	rest := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		rest <- string(data)
	}()
	select {
	case tail := <-rest:
		if strings.Contains(tail, endMarker) {
			t.Errorf("the stream ended with an end event, but the run did not end:\n%s", tail)
		}
	case <-time.After(runTestTimeout):
		t.Fatal("the open stream was not ended by the shutdown")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v, want nil", err)
		}
	case <-time.After(runTestTimeout):
		t.Fatal("run did not return while a stream was open")
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), runTestTimeout); err == nil {
		_ = conn.Close()
		t.Error("the listener still takes connections after the shutdown")
	}
}

// liveHub is a hub over a fake cluster, following a running run.
func liveHub(t *testing.T) (*scenario.Run, *stream.Hub) {
	t.Helper()
	world, err := scenario.New()
	if err != nil {
		t.Fatal(err)
	}
	live, err := world.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	src, err := world.Source()
	if err != nil {
		t.Fatal(err)
	}
	hub, err := stream.New(stream.Config{Source: src, Logger: slog.New(slog.DiscardHandler), PollInterval: followPoll, RereadDelay: followPoll})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	return live, hub
}

// openStream loads the run's snapshot and opens its stream from there:
// without a cursor of the run's epoch a stream starts with a reset.
func openStream(t *testing.T, ctx context.Context, base string, key contract.RunKey) *http.Response {
	t.Helper()
	res := get(t, ctx, base+key.SnapshotPath())
	var snap contract.Snapshot
	if err := json.NewDecoder(res.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	after := stream.Cursor{Epoch: snap.Stream.Epoch, Seq: snap.Stream.LastSeq}.String()
	events := get(t, ctx, base+key.EventsPath()+"?after="+after)
	if events.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", events.StatusCode)
	}
	return events
}

// oneShot sends each request on a connection of its own. A client that
// reuses connections can leave a fresh one open that never sends a request,
// which the server's shutdown waits for up to five seconds.
var oneShot = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

// get is GET url under ctx.
func get(t *testing.T, ctx context.Context, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := oneShot.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// awaitLine reads lines until one starts with prefix, failing when the
// stream ends or runTestTimeout passes first.
func awaitLine(t *testing.T, r *bufio.Reader, prefix string) {
	t.Helper()
	found := make(chan error, 1)
	go func() {
		for {
			line, err := r.ReadString('\n')
			if strings.HasPrefix(line, prefix) {
				found <- nil
				return
			}
			if err != nil {
				found <- err
				return
			}
		}
	}()
	select {
	case err := <-found:
		if err != nil {
			t.Fatalf("waiting for %q: %v", prefix, err)
		}
	case <-time.After(runTestTimeout):
		t.Fatalf("no %q line in %s", prefix, runTestTimeout)
	}
}
