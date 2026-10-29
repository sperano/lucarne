package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// RunLister lists the runs of the allowlisted namespaces, newest first and
// bounded; *kube.Source is one.
type RunLister interface {
	ListRuns(ctx context.Context) (contract.RunList, error)
}

var _ RunLister = (*kube.Source)(nil)

// listForbiddenMessage is the run list's answer when the Kubernetes API
// refused to list Tasks; the other errors share the run endpoints'.
const listForbiddenMessage = "the Kubernetes API refused to let the server list the runs"

// runList answers GET /api/v1/runs: the runs the UI can search and open.
func (s *server) runList(w http.ResponseWriter, r *http.Request) {
	if s.catalog == nil {
		s.writeError(w, http.StatusNotImplemented, contract.ErrorNotImplemented, noSourceMessage)
		return
	}
	list, err := s.catalog.ListRuns(r.Context())
	if err != nil {
		s.listError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// listError answers a run list that could not be read. The error itself is
// logged, never sent; a client that went away gets no answer.
func (s *server) listError(w http.ResponseWriter, r *http.Request, err error) {
	if ctx := r.Context(); ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		s.logger.Debug("list runs: client gone", "error", err)
		return
	}
	switch {
	case errors.Is(err, kube.ErrForbidden):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, listForbiddenMessage)
	case errors.Is(err, kube.ErrUnavailable):
		s.writeError(w, http.StatusServiceUnavailable, contract.ErrorUnavailable, unavailableMessage)
	default:
		s.writeError(w, http.StatusInternalServerError, contract.ErrorInternal, internalErrorMessage)
	}
	s.logger.Warn("list runs", "error", err)
}
