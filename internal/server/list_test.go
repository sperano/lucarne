package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
)

// fakeCatalog answers ListRuns with list, or with err.
type fakeCatalog struct {
	list contract.RunList
	err  error
}

func (f fakeCatalog) ListRuns(ctx context.Context) (contract.RunList, error) {
	if f.err != nil {
		return contract.RunList{}, f.err
	}
	return f.list, ctx.Err()
}

func catalogServer(catalog RunLister) http.Handler {
	return New(builtAssets(), Options{Catalog: catalog, Logger: slog.New(slog.DiscardHandler)})
}

func TestRunListEndpoint(t *testing.T) {
	t.Parallel()
	created := contract.Timestamp{Time: time.Date(2026, 3, 4, 10, 15, 0, 0, time.UTC)}
	want := contract.RunList{
		Schema: contract.SchemaVersion,
		Runs: []contract.Run{{
			Key: runKey1, TaskName: "fix-login-redirect", Runner: contract.RunnerClaudeCode,
			Phase: contract.PhaseRunning, Created: &created,
		}},
		OmittedRuns: 3,
	}
	res := get(t, catalogServer(fakeCatalog{list: want}), contract.RunsPath)
	data := body(t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET %s = %d %q: %s", contract.RunsPath, res.StatusCode, res.Header.Get("Cache-Control"), data)
	}
	if err := contract.Validate(contract.DefRunList, []byte(data)); err != nil {
		t.Fatal(err)
	}
	var got contract.RunList
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Runs) != 1 || got.Runs[0].Key != runKey1 || got.OmittedRuns != want.OmittedRuns {
		t.Errorf("list = %+v, want %+v", got, want)
	}
}

// TestEmptyRunListIsValid checks that a list with no run is an empty
// array, which the schema requires, not null.
func TestEmptyRunListIsValid(t *testing.T) {
	t.Parallel()
	res := get(t, catalogServer(fakeCatalog{list: contract.RunList{Schema: contract.SchemaVersion}}), contract.RunsPath)
	if err := contract.Validate(contract.DefRunList, []byte(body(t, res))); err != nil {
		t.Error(err)
	}
}

func TestRunListWithoutCatalog(t *testing.T) {
	t.Parallel()
	checkAPIErrors(t, []apiErrorCase{
		{http.MethodGet, contract.RunsPath, http.StatusNotImplemented, contract.ErrorNotImplemented},
		{http.MethodPost, contract.RunsPath, http.StatusMethodNotAllowed, contract.ErrorMethodNotAllowed},
	})
}

func TestRunListErrorsNameNoSecret(t *testing.T) {
	t.Parallel()
	const secret = "token=hunter2 at internal.example.com"
	tests := map[string]struct {
		err    error
		status int
		code   contract.ErrorCode
	}{
		"unavailable": {fmt.Errorf("%w: dial %s", kube.ErrUnavailable, secret), http.StatusServiceUnavailable, contract.ErrorUnavailable},
		"forbidden":   {fmt.Errorf("%w: %s", kube.ErrForbidden, secret), http.StatusServiceUnavailable, contract.ErrorUnavailable},
		"other":       {errors.New(secret), http.StatusInternalServerError, contract.ErrorInternal},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := get(t, catalogServer(fakeCatalog{err: tt.err}), contract.RunsPath)
			data := body(t, res)
			if res.StatusCode != tt.status || !strings.Contains(data, `"code":"`+string(tt.code)+`"`) {
				t.Errorf("GET %s: %d %s, want %d %s", contract.RunsPath, res.StatusCode, data, tt.status, tt.code)
			}
			if err := contract.Validate(contract.DefErrorResponse, []byte(data)); err != nil {
				t.Error(err)
			}
			if strings.Contains(data, "hunter2") || strings.Contains(data, "internal.example") {
				t.Errorf("answered the source's error: %s", data)
			}
		})
	}
}

// TestRunListOfAGoneClientIsNoWarning checks that a list given up by its
// client gets no answer and only a debug log.
func TestRunListOfAGoneClientIsNoWarning(t *testing.T) {
	t.Parallel()
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := New(builtAssets(), Options{Catalog: fakeCatalog{err: fmt.Errorf("list tasks: %w", context.Canceled)}, Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, contract.RunsPath, nil))
	if rec.Body.Len() != 0 || strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("answered %d %q, logged %q", rec.Code, rec.Body, logs.String())
	}
}
