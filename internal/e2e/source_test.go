package e2e

import (
	"net/http"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

// sourceLine is the source line of the event id of the run with key,
// which must be a valid 200.
func (s *stack) sourceLine(key contract.RunKey, id string) contract.SourceLine {
	s.t.Helper()
	r := s.get(key.SourcePath(id))
	if r.Status != http.StatusOK {
		s.t.Fatalf("GET %s: status %d: %s", r.URL, r.Status, truncateForLog(r.Body))
	}
	checkJSONHeaders(s.t, r)
	if err := contract.Validate(contract.DefSourceLine, r.Body); err != nil {
		s.t.Fatalf("GET %s: %v", r.URL, err)
	}
	var line contract.SourceLine
	mustUnmarshal(s.t, r.Body, &line)
	return line
}

func TestSourceLinesAreTheFixturesOwnLines(t *testing.T) {
	st := newStack(t)
	for _, f := range fixtureRuns(t, st.world) {
		t.Run(f.runner+"/"+f.scenario, func(t *testing.T) {
			st := st.with(t)
			data, err := fixtures.Read(f.file)
			if err != nil {
				t.Fatal(err)
			}
			logged := scenario.LogLines(string(data))
			snap := st.snapshot(f.run.Key)
			for _, ev := range snap.Events {
				if ev.Kind == contract.KindGap {
					st.apiError(st.get(f.run.Key.SourcePath(ev.ID)), http.StatusNotFound, contract.ErrorSourceNotFound)
					continue
				}
				line := st.sourceLine(f.run.Key, ev.ID)
				if line.Line != ev.Source.Line || line.Attempt != ev.Attempt {
					t.Fatalf("event %s: source line %d of %s", ev.ID, line.Line, line.Attempt)
				}
				if want := logged[ev.Source.Line-1]; line.Text != want || line.Bytes != int64(len(want)) {
					t.Fatalf("event %s: source %q (%d bytes), the fixture's line is %q", ev.ID, line.Text, line.Bytes, want)
				}
			}
		})
	}
}

func TestSourceLinesAreServedOnlyForTheirRun(t *testing.T) {
	st := newStack(t)
	own := st.world.Run(scenario.ForeignPod).Key
	snap := st.snapshot(own)
	if len(snap.Events) == 0 {
		t.Fatal("the run has no event: the check is vacuous")
	}
	id := snap.Events[0].ID
	st.sourceLine(own, id)

	other := fixtureRuns(t, st.world)[0].run.Key
	st.snapshot(other)
	st.apiError(st.get(other.SourcePath(id)), http.StatusNotFound, contract.ErrorSourceNotFound)
	private := st.world.Run(scenario.PrivateRun).Key
	st.apiError(st.get(private.SourcePath(id)), http.StatusNotFound, contract.ErrorRunNotFound)
	st.apiError(st.get(own.SourcePath("not-an-event-id")), http.StatusBadRequest, contract.ErrorInvalidEventID)
	st.assertNeverSent(scenario.ForeignMarker, scenario.PrivateMarker)
}
