package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
)

const (
	testIndex = "<!doctype html><title>Lucarne</title>"
	testAsset = "console.log('lucarne')"
)

var runKey1 = contract.RunKey{Cluster: "example", Namespace: "agents", TaskUID: "3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"}

func builtAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(testIndex)},
		"assets/index-abc.js":  {Data: []byte(testAsset)},
		"assets/nested/a.js":   {Data: []byte(testAsset)},
		".gitkeep":             {Data: nil},
		"assets/../secret.txt": {Data: []byte("never served")},
	}
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newTestServer(assets fstest.MapFS) http.Handler {
	return New(assets, Options{Logger: slog.New(slog.DiscardHandler)})
}

func TestHealth(t *testing.T) {
	t.Parallel()
	res := get(t, newTestServer(builtAssets()), "/healthz")
	if res.StatusCode != http.StatusOK || body(t, res) != "ok\n" {
		t.Errorf("GET /healthz = %d", res.StatusCode)
	}
}

func TestReadiness(t *testing.T) {
	t.Parallel()
	if res := get(t, newTestServer(builtAssets()), "/readyz"); res.StatusCode != http.StatusOK || body(t, res) != "ready\n" {
		t.Errorf("GET /readyz = %d, want 200 by default", res.StatusCode)
	}
	var ready atomic.Bool
	ready.Store(true)
	h := New(builtAssets(), Options{Logger: slog.New(slog.DiscardHandler), Ready: ready.Load})
	if res := get(t, h, "/readyz"); res.StatusCode != http.StatusOK {
		t.Errorf("GET /readyz = %d while ready", res.StatusCode)
	}
	ready.Store(false)
	res := get(t, h, "/readyz")
	if res.StatusCode != http.StatusServiceUnavailable || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("GET /readyz = %d, Cache-Control %q while draining", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	if res := get(t, h, "/healthz"); res.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz = %d while draining: the server is alive", res.StatusCode)
	}
}

func TestContractInfoIsValid(t *testing.T) {
	t.Parallel()
	res := get(t, newTestServer(builtAssets()), contract.ContractPath)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if err := contract.Validate(contract.DefContractInfo, []byte(body(t, res))); err != nil {
		t.Error(err)
	}
}

// encodedTaskUID is runKey1's Task UID with its "2" percent-encoded: it
// decodes to a valid key, but is not in canonical form.
var encodedTaskUID = strings.Replace(runKey1.TaskUID, "2", "%32", 1)

type apiErrorCase struct {
	method string
	path   string
	status int
	code   contract.ErrorCode
}

func TestRunEndpointsValidateTheKey(t *testing.T) {
	t.Parallel()
	checkAPIErrors(t, []apiErrorCase{
		{http.MethodGet, runKey1.SnapshotPath(), http.StatusNotImplemented, contract.ErrorNotImplemented},
		{http.MethodGet, runKey1.EventsPath(), http.StatusNotImplemented, contract.ErrorNotImplemented},
		{http.MethodGet, "/api/v1/runs/example/agents/example-claude-42", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, "/api/v1/runs/Example/agents/" + runKey1.TaskUID + "/events", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, "/api/v1/runs/example/agents/" + encodedTaskUID, http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, "/api/v1/runs/example/agents/" + encodedTaskUID + "/events", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, "/api/v1/runs/example/agents", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, runKey1.SnapshotPath() + "/", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, runKey1.EventsPath() + "/extra", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{http.MethodGet, "/api/v1/runs/", http.StatusBadRequest, contract.ErrorInvalidRunKey},
	})
}

func TestUnknownAPIRequestsAnswerJSON(t *testing.T) {
	t.Parallel()
	checkAPIErrors(t, []apiErrorCase{
		{http.MethodGet, "/api/v1", http.StatusNotFound, contract.ErrorNotFound},
		{http.MethodGet, "/api/v1/", http.StatusNotFound, contract.ErrorNotFound},
		{http.MethodGet, "/api/v1/missing", http.StatusNotFound, contract.ErrorNotFound},
		{http.MethodGet, contract.ContractPath + "/", http.StatusNotFound, contract.ErrorNotFound},
		{http.MethodPost, contract.ContractPath, http.StatusMethodNotAllowed, contract.ErrorMethodNotAllowed},
		{http.MethodDelete, runKey1.SnapshotPath(), http.StatusMethodNotAllowed, contract.ErrorMethodNotAllowed},
		{http.MethodPost, "/api/v1/runs/example", http.StatusMethodNotAllowed, contract.ErrorMethodNotAllowed},
	})
}

func checkAPIErrors(t *testing.T, cases []apiErrorCase) {
	t.Helper()
	h := newTestServer(builtAssets())
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
			res := rec.Result()
			data := body(t, res)
			if res.StatusCode != c.status {
				t.Errorf("status %d, want %d", res.StatusCode, c.status)
			}
			if c.status == http.StatusMethodNotAllowed && res.Header.Get("Allow") != allowedMethods {
				t.Errorf("Allow %q, want %q", res.Header.Get("Allow"), allowedMethods)
			}
			if err := contract.Validate(contract.DefErrorResponse, []byte(data)); err != nil {
				t.Fatal(err)
			}
			var got contract.ErrorResponse
			if err := json.Unmarshal([]byte(data), &got); err != nil || got.Error.Code != c.code {
				t.Errorf("code %q (%v), want %q", got.Error.Code, err, c.code)
			}
		})
	}
}

func TestDeepLinksServeTheUI(t *testing.T) {
	t.Parallel()
	h := newTestServer(builtAssets())
	cases := map[string]int{
		"/":                                      http.StatusOK,
		runKey1.LinkPath():                       http.StatusOK,
		"/runs/example/agents/example-claude-42": http.StatusNotFound,
		runKey1.LinkPath() + "/":                 http.StatusNotFound,
		runKey1.LinkPath() + "/extra":            http.StatusNotFound,
		"/runs/example/" + runKey1.TaskUID:       http.StatusNotFound,
		"/runs/example/agents/" + encodedTaskUID: http.StatusNotFound,
		"/runs/":                                 http.StatusNotFound,
	}
	for path, status := range cases {
		res := get(t, h, path)
		if res.StatusCode != status || body(t, res) != testIndex {
			t.Errorf("GET %s = %d, want %d with the index", path, res.StatusCode, status)
		}
		if got := res.Header.Get("Cache-Control"); got != cacheIndex {
			t.Errorf("GET %s Cache-Control %q", path, got)
		}
	}
}

func TestAssets(t *testing.T) {
	t.Parallel()
	h := newTestServer(builtAssets())
	for _, path := range []string{"/assets/index-abc.js", "/assets/nested/a.js"} {
		res := get(t, h, path)
		if res.StatusCode != http.StatusOK || body(t, res) != testAsset {
			t.Fatalf("GET %s = %d", path, res.StatusCode)
		}
		if got := res.Header.Get("Cache-Control"); got != cacheAssets {
			t.Errorf("GET %s Cache-Control %q", path, got)
		}
	}
	// "/assets/../secret.txt" never reaches the asset handler: ServeMux cleans
	// the path and redirects to "/secret.txt", and its {file...} wildcard
	// cannot match a ".." segment. The case pins that net/http behavior; the
	// handler has no traversal guard of its own.
	for _, path := range []string{"/assets/missing.js", "/assets/nested", "/.gitkeep", "/index.html", "/assets/../secret.txt", "/elsewhere"} {
		res := get(t, h, path)
		if res.StatusCode == http.StatusOK {
			t.Errorf("GET %s = 200, want an error", path)
		}
		if got := res.Header.Get("Cache-Control"); got == cacheAssets {
			t.Errorf("GET %s cached as an asset", path)
		}
	}
}

func TestWithoutABuiltFrontend(t *testing.T) {
	t.Parallel()
	h := newTestServer(fstest.MapFS{".gitkeep": {Data: nil}})
	res := get(t, h, runKey1.LinkPath())
	if res.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body(t, res), "npm run build") {
		t.Errorf("status %d, want 503 naming the build", res.StatusCode)
	}
	if res := get(t, h, "/healthz"); res.StatusCode != http.StatusOK {
		t.Errorf("healthz %d without a frontend", res.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/", "/healthz", contract.ContractPath, "/missing"} {
		res := get(t, newTestServer(builtAssets()), path)
		if res.Header.Get("Content-Security-Policy") != contentSecurityPolicy ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("GET %s lacks the security headers", path)
		}
	}
}

func TestTruncateKeepsRunes(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("é", 10) // 2 bytes each
	got := truncate(s, 5)
	if !utf8.ValidString(got) || len(got) != 4 {
		t.Errorf("truncate = %q (%d bytes), want 4 valid bytes", got, len(got))
	}
	if truncate("short", 10) != "short" {
		t.Error("a short string was changed")
	}
}

func TestNilLoggerFallsBackToDefault(t *testing.T) {
	t.Parallel()
	h := New(fstest.MapFS{}, Options{})
	if res := get(t, h, runKey1.LinkPath()); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503 without a frontend", res.StatusCode)
	}
}

func TestEncodedPathIsAnInvalidKey(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/runs/example/agents/"+encodedTaskUID, nil)
	if err := validRunKey(r); !errors.Is(err, contract.ErrInvalidKey) {
		t.Errorf("err = %v, want ErrInvalidKey", err)
	}
}

func TestUnencodableResponseIsAnErrorResponse(t *testing.T) {
	t.Parallel()
	s := &server{logger: slog.New(slog.DiscardHandler)}
	rec := httptest.NewRecorder()
	s.writeJSON(rec, http.StatusOK, make(chan int))
	res := rec.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", res.StatusCode)
	}
	data := body(t, res)
	if err := contract.Validate(contract.DefErrorResponse, []byte(data)); err != nil {
		t.Fatal(err)
	}
	var got contract.ErrorResponse
	if err := json.Unmarshal([]byte(data), &got); err != nil || got.Error.Code != contract.ErrorInternal {
		t.Errorf("code %q (%v), want %q", got.Error.Code, err, contract.ErrorInternal)
	}
}
