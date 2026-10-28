package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// eventsSuffix ends the path of a run's SSE stream.
const eventsSuffix = "/events"

// streamGate lets a test cut the browser's live streams as a network
// outage would. Chromium's offline emulation leaves an open EventSource
// connected, so the cut is made here: cut ends every stream open now and
// holds the reconnects that follow, unanswered, until restore lets them
// through to the server with their Last-Event-ID.
type streamGate struct {
	mu   sync.Mutex
	open map[uint64]context.CancelFunc
	next uint64
	// up is closed by restore; nil while the streams are not cut.
	up chan struct{}
}

func newStreamGate() *streamGate {
	return &streamGate{open: map[uint64]context.CancelFunc{}}
}

// wrap passes every request to next, a stream once the gate lets it.
func (g *streamGate) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, eventsSuffix) {
			next.ServeHTTP(w, r)
			return
		}
		if !g.wait(r.Context()) {
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		id := g.add(cancel)
		defer g.remove(id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// cut ends the open streams and holds new ones until restore.
func (g *streamGate) cut() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up == nil {
		g.up = make(chan struct{})
	}
	for _, cancel := range g.open {
		cancel()
	}
}

// restore lets the held streams, and new ones, through.
func (g *streamGate) restore() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil {
		close(g.up)
		g.up = nil
	}
}

// wait blocks while the streams are cut; false when the client left first.
func (g *streamGate) wait(ctx context.Context) bool {
	g.mu.Lock()
	up := g.up
	g.mu.Unlock()
	if up == nil {
		return true
	}
	select {
	case <-up:
		return true
	case <-ctx.Done():
		return false
	}
}

// count is how many streams are open: served, not held, not ended.
func (g *streamGate) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.open)
}

func (g *streamGate) add(cancel context.CancelFunc) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	g.open[g.next] = cancel
	return g.next
}

func (g *streamGate) remove(id uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.open, id)
}
