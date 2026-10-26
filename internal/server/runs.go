package server

import (
	"errors"
	"net/http"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream"
)

// Messages of the run endpoints' errors. They are fixed: an error of the
// source can name hosts or credentials, so it is only logged.
const (
	noSourceMessage    = "this server reads no cluster: start it with -cluster and -namespace"
	runNotFoundMessage = "no such run, or it expired"
	busyMessage        = "the server follows as many runs as it can: try again later"
	closedMessage      = "the server is shutting down"
	forbiddenMessage   = "the Kubernetes API refused to let the server read this run"
	unavailableMessage = "the Kubernetes API did not answer"
)

// snapshot answers GET /api/v1/runs/{cluster}/{namespace}/{taskUID}.
func (s *server) snapshot(w http.ResponseWriter, r *http.Request) {
	key, ok := s.servedRunKey(w, r)
	if !ok {
		return
	}
	body, err := s.runs.Snapshot(r.Context(), key)
	if err != nil {
		s.runError(w, r, key, err)
		return
	}
	s.writeEncoded(w, http.StatusOK, append(body, '\n'))
}

// events answers GET /api/v1/runs/{cluster}/{namespace}/{taskUID}/events,
// the run's SSE stream. A cursor at or after the run's end is answered
// 204, which tells an EventSource not to reconnect.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	key, ok := s.servedRunKey(w, r)
	if !ok {
		return
	}
	sub, err := s.runs.Subscribe(r.Context(), key, cursorOf(r))
	if err != nil {
		s.runError(w, r, key, err)
		return
	}
	defer sub.Close()
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	if sub.Finished {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.Set("Content-Type", "text/event-stream")
	// Proxies that buffer responses (nginx) must not hold the stream back.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	s.serveStream(r.Context(), newSSEWriter(w), sub)
}

// servedRunKey is the request's run key, after answering a request that
// has none or that this server cannot serve.
func (s *server) servedRunKey(w http.ResponseWriter, r *http.Request) (contract.RunKey, bool) {
	key, err := runKey(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, contract.ErrorInvalidRunKey, err.Error())
		return key, false
	}
	if s.runs == nil {
		s.writeError(w, http.StatusNotImplemented, contract.ErrorNotImplemented, noSourceMessage)
		return key, false
	}
	return key, true
}

// cursorOf is the stream cursor of a request: Last-Event-ID, which a
// reconnecting browser sends, else the after parameter.
func cursorOf(r *http.Request) string {
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		return id
	}
	return r.URL.Query().Get("after")
}

// runError answers a run that could not be served. The error itself is
// logged, never sent. A request given up while its run was being opened
// gets no answer: its client is gone.
func (s *server) runError(w http.ResponseWriter, r *http.Request, key contract.RunKey, err error) {
	if ctx := r.Context(); ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		s.logger.Debug("serve run: client gone", "run", key.LinkPath(), "error", err)
		return
	}
	switch {
	case errors.Is(err, kube.ErrRunNotFound):
		s.writeError(w, http.StatusNotFound, contract.ErrorRunNotFound, runNotFoundMessage)
		return
	case errors.Is(err, stream.ErrBusy):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, busyMessage)
	case errors.Is(err, stream.ErrClosed):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, closedMessage)
	case errors.Is(err, kube.ErrForbidden):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, forbiddenMessage)
	case errors.Is(err, kube.ErrUnavailable):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, unavailableMessage)
	default:
		s.writeError(w, http.StatusInternalServerError, contract.ErrorInternal, internalErrorMessage)
	}
	s.logger.Warn("serve run", "run", key.LinkPath(), "error", err)
}
