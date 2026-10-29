// Package server is Lucarne's HTTP server: the API of contract version 1 and
// the embedded frontend, with its deep links.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/stream"
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

	drainingMessage = "draining: the server is shutting down\n"

	notBuiltMessage = "frontend not built: run `npm run build` in web/\n"

	notARunPathMessage = "not a run path: want runs/{cluster}/{namespace}/{taskUID}, optionally followed by /events or /source/{eventID}"

	// allowedMethods is the Allow header of a 405: every endpoint is GET.
	allowedMethods = "GET, HEAD"

	// internalErrorMessage is the message of the 500 sent when a response
	// cannot be encoded.
	internalErrorMessage = "internal error"
)

// errEncodedPath wraps contract.ErrInvalidKey, as every other run key error
// does.
var errEncodedPath = fmt.Errorf("%w: run key is percent-encoded: its parts must be in canonical form", contract.ErrInvalidKey)

// internalErrorBody is the ErrorResponse sent when a response cannot be
// encoded, so that even that failure answers the contract.
var internalErrorBody = mustEncodeInternalError()

// mustEncodeInternalError encodes the 500 fallback once, at init, from the
// contract's own types.
func mustEncodeInternalError() []byte {
	body, err := json.Marshal(contract.ErrorResponse{Error: contract.ErrorBody{
		Code:    contract.ErrorInternal,
		Message: internalErrorMessage,
	}})
	if err != nil {
		panic(fmt.Sprintf("server: encode the internal error body: %v", err))
	}
	return append(body, '\n')
}

// Options are what a server serves runs from and how.
type Options struct {
	// Runs serves run snapshots and streams; without it the run endpoints
	// answer 501.
	Runs *stream.Hub
	// Catalog lists the runs the UI can search; without it the run list
	// answers 501.
	Catalog RunLister
	// Heartbeat is how long a stream may be idle before a keepalive
	// comment; zero means DefaultHeartbeat.
	Heartbeat time.Duration
	// Logger is where the server logs; nil means slog.Default().
	Logger *slog.Logger
	// Ready reports whether the server takes new requests, as /readyz
	// says; nil means always. It turns false while the server drains.
	Ready func() bool
}

// New returns the handler serving the API and the frontend in assets, the
// contents of web/dist.
func New(assets fs.FS, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	index, err := fs.ReadFile(assets, indexFile)
	if err != nil {
		logger.Warn("serving without a frontend", "error", err)
		index = nil
	}
	heartbeat := opts.Heartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeat
	}
	ready := opts.Ready
	if ready == nil {
		ready = func() bool { return true }
	}
	s := &server{assets: assets, index: index, logger: logger, runs: opts.Runs, catalog: opts.Catalog, heartbeat: heartbeat, ready: ready}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.readiness)
	mux.HandleFunc("GET "+contract.ContractPath, s.contractInfo)
	mux.HandleFunc("GET "+contract.RunsPath, s.runList)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/{cluster}/{namespace}/{taskUID}", s.snapshot)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/{cluster}/{namespace}/{taskUID}/events", s.events)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/{cluster}/{namespace}/{taskUID}/source/{eventID}", s.source)
	mux.HandleFunc("GET "+contract.APIPrefix+"/runs/", s.notARunPath)
	mux.HandleFunc(contract.APIPrefix, s.apiNotFound)
	mux.HandleFunc(contract.APIPrefix+"/", s.apiNotFound)
	mux.HandleFunc("GET /"+assetsDir+"/{file...}", s.asset)
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /runs/{cluster}/{namespace}/{taskUID}", s.runPage)
	mux.HandleFunc("GET /runs/", s.notARunLink)
	return securityHeaders(mux)
}

type server struct {
	assets    fs.FS
	index     []byte
	logger    *slog.Logger
	runs      *stream.Hub
	catalog   RunLister
	heartbeat time.Duration
	ready     func() bool
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	s.write(w, []byte("ok\n"))
}

// readiness answers whether the server takes new requests: 503 while it
// drains, so that the Service stops routing to it. It calls no source, so
// that a Kubernetes API that does not answer leaves the UI reachable.
func (s *server) readiness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if !s.ready() {
		w.WriteHeader(http.StatusServiceUnavailable)
		s.write(w, []byte(drainingMessage))
		return
	}
	s.write(w, []byte("ready\n"))
}

func (s *server) contractInfo(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, contract.ContractInfo{
		Schema: contract.SchemaVersion,
		Limits: contract.DefaultLimits(),
	})
}

// notARunPath answers a path under the runs endpoint of another shape: a
// trailing slash, too few or too many parts.
func (s *server) notARunPath(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, http.StatusBadRequest, contract.ErrorInvalidRunKey, notARunPathMessage)
}

// apiNotFound answers every other API request, so that each API error is an
// ErrorResponse. Every endpoint is GET, so another method is never allowed.
func (s *server) apiNotFound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", allowedMethods)
		s.writeError(w, http.StatusMethodNotAllowed, contract.ErrorMethodNotAllowed, "API endpoints answer only GET")
		return
	}
	s.writeError(w, http.StatusNotFound, contract.ErrorNotFound, "no API endpoint at this path")
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
	if err := validRunKey(r); err != nil {
		status = http.StatusNotFound
	}
	s.writeIndex(w, status)
}

// notARunLink serves the UI, which says what is wrong, with status 404 for a
// path under /runs/ of another shape: a trailing slash, too few or too many
// parts.
func (s *server) notARunLink(w http.ResponseWriter, _ *http.Request) {
	s.writeIndex(w, http.StatusNotFound)
}

func (s *server) writeIndex(w http.ResponseWriter, status int) {
	if s.index == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		s.write(w, []byte(notBuiltMessage))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", cacheIndex)
	w.WriteHeader(status)
	s.write(w, s.index)
}

// validRunKey checks the run key in the path.
func validRunKey(r *http.Request) error {
	_, err := runKey(r)
	return err
}

// runKey is the run key in the path. A canonical key has no character that
// needs escaping, so a percent-encoded path is rejected: the mux decodes
// it, the UI does not, and both must read a URL as one key.
func runKey(r *http.Request) (contract.RunKey, error) {
	if r.URL.EscapedPath() != r.URL.Path {
		return contract.RunKey{}, errEncodedPath
	}
	key := contract.RunKey{
		Cluster:   r.PathValue("cluster"),
		Namespace: r.PathValue("namespace"),
		TaskUID:   r.PathValue("taskUID"),
	}
	return key, key.Validate()
}

func (s *server) writeError(w http.ResponseWriter, status int, code contract.ErrorCode, message string) {
	s.writeJSON(w, status, contract.ErrorResponse{Error: contract.ErrorBody{Code: code, Message: truncate(message, contract.MaxLabelBytes)}})
}

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("encode response", "error", err)
		status, body = http.StatusInternalServerError, internalErrorBody
	} else {
		body = append(body, '\n')
	}
	s.writeEncoded(w, status, body)
}

// writeEncoded sends an encoded JSON document.
func (s *server) writeEncoded(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	s.write(w, body)
}

// write sends a response body. A failure means the client went away, so it
// is logged at debug level only.
func (s *server) write(w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		s.logger.Debug("write response", "error", err)
	}
}

// truncate cuts s to at most limit bytes without splitting a UTF-8 rune.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
