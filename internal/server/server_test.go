package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	return New(assets, slog.New(slog.DiscardHandler))
}

func TestHealth(t *testing.T) {
	res := get(t, newTestServer(builtAssets()), "/healthz")
	if res.StatusCode != http.StatusOK || body(t, res) != "ok\n" {
		t.Errorf("GET /healthz = %d", res.StatusCode)
	}
}

func TestContractInfoIsValid(t *testing.T) {
	res := get(t, newTestServer(builtAssets()), contract.ContractPath)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if err := contract.Validate(contract.DefContractInfo, []byte(body(t, res))); err != nil {
		t.Error(err)
	}
}

func TestRunEndpointsValidateTheKey(t *testing.T) {
	h := newTestServer(builtAssets())
	cases := []struct {
		path   string
		status int
		code   string
	}{
		{runKey1.SnapshotPath(), http.StatusNotImplemented, contract.ErrorNotImplemented},
		{runKey1.EventsPath(), http.StatusNotImplemented, contract.ErrorNotImplemented},
		{"/api/v1/runs/example/agents/example-claude-42", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"/api/v1/runs/Example/agents/" + runKey1.TaskUID + "/events", http.StatusBadRequest, contract.ErrorInvalidRunKey},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			res := get(t, h, c.path)
			data := body(t, res)
			if res.StatusCode != c.status {
				t.Errorf("status %d, want %d", res.StatusCode, c.status)
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
	h := newTestServer(builtAssets())
	cases := map[string]int{
		"/":                                      http.StatusOK,
		runKey1.LinkPath():                       http.StatusOK,
		"/runs/example/agents/example-claude-42": http.StatusNotFound,
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
	h := newTestServer(builtAssets())
	res := get(t, h, "/assets/index-abc.js")
	if res.StatusCode != http.StatusOK || body(t, res) != testAsset {
		t.Fatalf("asset status %d", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != cacheAssets {
		t.Errorf("Cache-Control %q", got)
	}
	for _, path := range []string{"/assets/missing.js", "/.gitkeep", "/index.html", "/assets/../secret.txt", "/elsewhere"} {
		if res := get(t, h, path); res.StatusCode == http.StatusOK {
			t.Errorf("GET %s = 200, want an error", path)
		}
	}
}

func TestWithoutABuiltFrontend(t *testing.T) {
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
	for _, path := range []string{"/", "/healthz", contract.ContractPath, "/missing"} {
		res := get(t, newTestServer(builtAssets()), path)
		if res.Header.Get("Content-Security-Policy") != contentSecurityPolicy ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("GET %s lacks the security headers", path)
		}
	}
}

func TestTruncateKeepsRunes(t *testing.T) {
	s := strings.Repeat("é", 10) // 2 bytes each
	got := truncate(s, 5)
	if !utf8.ValidString(got) || len(got) != 4 {
		t.Errorf("truncate = %q (%d bytes), want 4 valid bytes", got, len(got))
	}
	if truncate("short", 10) != "short" {
		t.Error("a short string was changed")
	}
}
