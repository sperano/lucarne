// Package server is Lucarne's HTTP server: the API of contract version 1 and
// the embedded frontend, with its deep links.
package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/sperano/lucarne/contract"
)

const (
	indexFile = "index.html"
	assetsDir = "assets"

	// Hashed asset names change with their content; index.html must not
	// be cached, so a deployment takes effect on the next load.
	cacheAssets = "public, max-age=31536000, immutable"
	cacheIndex  = "no-cache"

	// contentSecurityPolicy allows only same-origin resources: no inline
	// scripts, no eval, no framing.
	contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
		"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
		"form-action 'none'; frame-ancestors 'none'"

	notBuiltMessage = "frontend not built: run `npm run build` in web/\n"
)

// New returns the handler serving the API and the frontend in assets, the
// contents of web/dist.
func New(assets fs.FS, logger *slog.Logger) http.Handler {
	index, err := fs.ReadFile(assets, indexFile)
	if err != nil {
		logger.Warn("serving without a frontend", "error", err)
		index = nil
	}
	s := &server{assets: assets, index: index, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET "+contract.ContractPath, s.contractInfo)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/{cluster}/{namespace}/{taskUID}", s.runNotImplemented)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/{cluster}/{namespace}/{taskUID}/events", s.runNotImplemented)
	mux.HandleFunc("GET /"+assetsDir+"/{file}", s.asset)
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /runs/{cluster}/{namespace}/{taskUID}", s.runPage)
	return securityHeaders(mux)
}

type server struct {
	assets fs.FS
	index  []byte
	logger *slog.Logger
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *server) contractInfo(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, contract.ContractInfo{
		Schema: contract.SchemaVersion,
		Limits: contract.DefaultLimits(),
	})
}

// runNotImplemented validates the run key as the real endpoints will, then
// says the endpoint does not exist yet.
func (s *server) runNotImplemented(w http.ResponseWriter, r *http.Request) {
	if err := runKey(r).Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, contract.ErrorInvalidRunKey, err.Error())
		return
	}
	s.writeError(w, http.StatusNotImplemented, contract.ErrorNotImplemented, "run snapshots and streams are not implemented yet")
}

func (s *server) asset(w http.ResponseWriter, r *http.Request) {
	name := assetsDir + "/" + r.PathValue("file")
	// Checked before setting the immutable Cache-Control header, so that a
	// missing asset's 404 is not cached for a year.
	if info, err := fs.Stat(s.assets, name); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", cacheAssets)
	http.ServeFileFS(w, r, s.assets, name)
}

func (s *server) home(w http.ResponseWriter, _ *http.Request) {
	s.writeIndex(w, http.StatusOK)
}

// runPage serves the UI for a deep link. A malformed key still gets the UI,
// which explains the problem, with status 404.
func (s *server) runPage(w http.ResponseWriter, r *http.Request) {
	status := http.StatusOK
	if err := runKey(r).Validate(); err != nil {
		status = http.StatusNotFound
	}
	s.writeIndex(w, status)
}

func (s *server) writeIndex(w http.ResponseWriter, status int) {
	if s.index == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(notBuiltMessage))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", cacheIndex)
	w.WriteHeader(status)
	_, _ = w.Write(s.index)
}

func runKey(r *http.Request) contract.RunKey {
	return contract.RunKey{
		Cluster:   r.PathValue("cluster"),
		Namespace: r.PathValue("namespace"),
		TaskUID:   r.PathValue("taskUID"),
	}
}

func (s *server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, contract.ErrorResponse{Error: contract.ErrorBody{Code: code, Message: truncate(message, contract.MaxLabelBytes)}})
}

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("encode response", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		s.logger.Debug("write response", "error", err)
	}
}

// truncate cuts s to at most limit bytes without splitting a UTF-8 rune.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
