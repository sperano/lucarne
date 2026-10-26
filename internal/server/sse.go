package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/stream"
)

const (
	// DefaultHeartbeat is how long a stream may stay idle before a
	// keepalive comment, so that proxies keep it open.
	DefaultHeartbeat = 15 * time.Second
	// sseWriteTimeout bounds one write to a stream: a client that takes
	// longer is gone, and its stream is released.
	sseWriteTimeout = 30 * time.Second
	keepalive       = ": keepalive\n\n"
)

// serveStream sends a subscription: its reset, or its replay, the live
// marker and every update after it, until the run ends, the client is gone
// or too slow, or the hub closes.
func (s *server) serveStream(ctx context.Context, sw *sseWriter, sub *stream.Subscription) {
	if sub.Reset != nil {
		_ = sw.send(sub.Reset)
		return
	}
	for _, u := range sub.Replay {
		if sw.send(u) != nil || u.Event == contract.SSEEnd {
			return
		}
	}
	if sw.send(sub.Live) != nil {
		return
	}
	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	idle := true
	for {
		if stopped(sw, sub) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
		case u := <-sub.Updates():
			if sw.send(u) != nil || u.Event == contract.SSEEnd {
				return
			}
			idle = false
		case <-ticker.C:
			if idle && sw.write([]byte(keepalive)) != nil {
				return
			}
			idle = true
		}
	}
}

// stopped reports whether sub is done, after sending the reset it ends
// with: checked before each update, so a client too slow gets nothing more.
func stopped(sw *sseWriter, sub *stream.Subscription) bool {
	select {
	case <-sub.Done():
		if reset := sub.Err(); reset != nil {
			_ = sw.send(reset)
		}
		return true
	default:
		return false
	}
}

// sseWriter writes Server-Sent Events, each flushed at once.
type sseWriter struct {
	w   http.ResponseWriter
	rc  *http.ResponseController
	buf bytes.Buffer
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	return &sseWriter{w: w, rc: http.NewResponseController(w)}
}

// send writes one update. Its data is one line of JSON.
func (sw *sseWriter) send(u *stream.Update) error {
	sw.buf.Reset()
	sw.buf.WriteString("event: " + u.Event + "\n")
	if u.ID != "" {
		sw.buf.WriteString("id: " + u.ID + "\n")
	}
	sw.buf.WriteString("data: ")
	sw.buf.Write(u.Data)
	sw.buf.WriteString("\n\n")
	return sw.write(sw.buf.Bytes())
}

// write sends b and flushes it, within sseWriteTimeout.
func (sw *sseWriter) write(b []byte) error {
	err := sw.rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := sw.w.Write(b); err != nil {
		return err
	}
	return sw.rc.Flush()
}
