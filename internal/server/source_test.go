package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

// finishedServer serves a finished run whose one attempt logged lines.
func finishedServer(t *testing.T, lines ...string) (*http.Server, string) {
	t.Helper()
	origin := streamtest.Origin("source")
	done := streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerTerminated)
	src := streamtest.NewSource(contract.PhaseSucceeded, contract.RunnerUnknown)
	src.SetAttempts(done)
	logged := make([]adapter.Line, 0, len(lines))
	for i, text := range lines {
		logged = append(logged, streamtest.Line(int64(i+1), text))
	}
	src.AddReads(done.ID, streamtest.Deliver(origin, logged, kube.EndComplete, ""))
	return liveServer(t, src).Config, origin
}

func TestSourceEndpoint(t *testing.T) {
	t.Parallel()
	const line = `{"type":"assistant","n":1}`
	srv, origin := finishedServer(t, line)
	attemptID := streamtest.Attempt(streamtest.PodUID, 0, contract.ContainerTerminated).ID
	// The snapshot opens the run and waits for its terminated attempt.
	if res := get(t, srv.Handler, streamtest.Key.SnapshotPath()); res.StatusCode != http.StatusOK {
		t.Fatalf("snapshot: %d", res.StatusCode)
	}

	res := get(t, srv.Handler, streamtest.Key.SourcePath(contract.EventID(attemptID, origin, 1, 0)))
	data := body(t, res)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("source: %d %s", res.StatusCode, data)
	}
	if err := contract.Validate(contract.DefSourceLine, []byte(data)); err != nil {
		t.Fatal(err)
	}
	var got contract.SourceLine
	mustUnmarshal(t, data, &got)
	if got.Text != line || got.Line != 1 || got.Attempt != attemptID || got.Origin != origin {
		t.Errorf("source line %+v", got)
	}

	for _, c := range []struct {
		path   string
		status int
		code   contract.ErrorCode
	}{
		{streamtest.Key.SourcePath(contract.GapEventID(attemptID, origin, 1, 0)), http.StatusNotFound, contract.ErrorSourceNotFound},
		{streamtest.Key.SourcePath(contract.EventID(attemptID, origin, 2, 0)), http.StatusNotFound, contract.ErrorSourceNotFound},
		{streamtest.Key.SourcePath("not-an-event"), http.StatusBadRequest, contract.ErrorInvalidEventID},
		{streamtest.Key.SourcePath(strings.Replace(contract.EventID(attemptID, origin, 1, 0), "1", "%31", 1)), http.StatusBadRequest, contract.ErrorInvalidRunKey},
	} {
		res := get(t, srv.Handler, c.path)
		data := body(t, res)
		if res.StatusCode != c.status || !strings.Contains(data, `"code":"`+string(c.code)+`"`) {
			t.Errorf("GET %s: %d %s, want %d %s", c.path, res.StatusCode, data, c.status, c.code)
		}
		if err := contract.Validate(contract.DefErrorResponse, []byte(data)); err != nil {
			t.Error(err)
		}
	}
}
