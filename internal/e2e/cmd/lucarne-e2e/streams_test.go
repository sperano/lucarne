package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	// gateTimeout bounds how long a test waits for a stream to start or end.
	gateTimeout = 5 * time.Second
	// heldFor is how long a held stream is checked not to start.
	heldFor = 100 * time.Millisecond
)

// blockingStream stands for the SSE handler: it reports that it started,
// then serves until its request's context ends.
func blockingStream(started chan<- struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	})
}

func serveAsync(h http.Handler, path string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}()
	return done
}

func receive[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(gateTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestStreamGateCutEndsOpenStreamsAndHoldsNewOnes(t *testing.T) {
	gate := newStreamGate()
	started := make(chan struct{}, 2)
	h := gate.wrap(blockingStream(started))

	first := serveAsync(h, "/api/v1/runs/example/agents/x"+eventsSuffix)
	receive(t, started, "the first stream to start")
	gate.cut()
	receive(t, first, "the cut to end the open stream")

	second := serveAsync(h, "/api/v1/runs/example/agents/x"+eventsSuffix)
	select {
	case <-started:
		t.Fatal("a stream started while the streams are cut")
	case <-time.After(heldFor):
	}
	gate.restore()
	receive(t, started, "the held stream to start after restore")
	gate.cut()
	receive(t, second, "the second cut to end the restored stream")
	gate.restore()
}

func TestStreamGatePassesOtherRequests(t *testing.T) {
	gate := newStreamGate()
	gate.cut()
	defer gate.restore()
	served := false
	h := gate.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs/example/agents/x", nil))
	if !served {
		t.Fatal("a snapshot request was held while the streams are cut")
	}
}

func TestStreamGateCountsTheStreamsItServes(t *testing.T) {
	gate := newStreamGate()
	started := make(chan struct{}, 1)
	h := gate.wrap(blockingStream(started))
	if n := gate.count(); n != 0 {
		t.Fatalf("count = %d before any stream, want 0", n)
	}
	served := serveAsync(h, "/api/v1/runs/example/agents/x"+eventsSuffix)
	receive(t, started, "the stream to start")
	if n := gate.count(); n != 1 {
		t.Fatalf("count = %d while a stream is served, want 1", n)
	}
	gate.cut()
	receive(t, served, "the cut to end the stream")
	gate.restore()
	if n := gate.count(); n != 0 {
		t.Fatalf("count = %d once the stream ended, want 0", n)
	}
}

func TestStreamGateDoesNotCountOtherRequests(t *testing.T) {
	gate := newStreamGate()
	var during int
	h := gate.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { during = gate.count() }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs/example/agents/x", nil))
	if during != 0 {
		t.Fatalf("count = %d during a snapshot request, want 0", during)
	}
}
