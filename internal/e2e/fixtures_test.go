package e2e

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

// goldenDirs maps a fixture's runner directory to the adapter package
// holding its golden files.
var goldenDirs = map[string]string{
	"claude-code": "claude",
	"codex":       "codex",
	"opencode":    "opencode",
}

// failedExitCode is the exit code of a failed fixture's agent.
const failedExitCode = 1

// fixtureRun is a fixture file and its run.
type fixtureRun struct {
	file, runner, scenario string
	run                    *scenario.Run
	failed                 bool
}

func fixtureRuns(t *testing.T, w *scenario.World) []fixtureRun {
	t.Helper()
	files, err := scenario.FixtureFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixture")
	}
	runs := make([]fixtureRun, 0, len(files))
	for _, file := range files {
		name := strings.TrimSuffix(path.Base(file), ".log")
		runner := strings.SplitN(file, "/", 2)[0]
		runs = append(runs, fixtureRun{
			file: file, runner: runner, scenario: name,
			run: w.Run(scenario.FixtureName(file)), failed: scenario.FixtureFailed(file),
		})
	}
	return runs
}

// goldenEvents are the events the runner's adapter makes of the fixture:
// its golden file.
func goldenEvents(t *testing.T, f fixtureRun) []contract.Event {
	t.Helper()
	dir, ok := goldenDirs[f.runner]
	if !ok {
		t.Fatalf("no golden directory known for runner %s", f.runner)
	}
	data, err := os.ReadFile(filepath.Join("..", "adapter", dir, "testdata", f.scenario+".golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var events []contract.Event
	mustUnmarshal(t, data, &events)
	return events
}

// comparable is an event reduced to what its log line decides. The golden
// files come from the adapter run alone, so what the stream, the source
// and the cluster add is left out: the seq (the stream numbers its
// attempt and run updates too), the attempt ID (the pod UID is random) and
// the log time (the container runtime stamps it). The event ID is kept
// as its line and part: the origin before them is a hash of the log's
// first line as the source read it, stamped, so it is not the adapter
// test's; the line and part say the source numbered the lines the same way.
func comparable(t *testing.T, ev contract.Event) string {
	t.Helper()
	parts := strings.Split(ev.ID, ":")
	if len(parts) < 3 {
		t.Fatalf("event ID %q is not attempt:origin:line:part", ev.ID)
	}
	ev.Seq, ev.Attempt, ev.Source.LogTime = 0, "", nil
	ev.ID = strings.Join(parts[len(parts)-2:], ":")
	return payloadJSON(t, ev)
}

func TestFixtureRunsMatchTheAdaptersGoldenFiles(t *testing.T) {
	st := newStack(t)
	for _, f := range fixtureRuns(t, st.world) {
		t.Run(f.runner+"/"+f.scenario, func(t *testing.T) {
			st := st.with(t)
			snap := st.snapshot(f.run.Key)
			checkFixtureSnapshot(t, f, snap)
			want, got := goldenEvents(t, f), snap.Events
			if len(got) != len(want) {
				t.Fatalf("%d events, the golden file has %d", len(got), len(want))
			}
			for i := range want {
				if g, w := comparable(t, got[i]), comparable(t, want[i]); g != w {
					t.Fatalf("event %d differs from the golden file:\n got %s\nwant %s", i+1, g, w)
				}
			}
			checkFixtureStream(st, f.run.Key, snap)
		})
	}
}

// checkFixtureSnapshot checks what a finished run's snapshot says besides
// its events.
func checkFixtureSnapshot(t *testing.T, f fixtureRun, snap contract.Snapshot) {
	t.Helper()
	if snap.Run.Key != f.run.Key {
		t.Errorf("run key %v, want %v", snap.Run.Key, f.run.Key)
	}
	wantPhase, wantExit := contract.PhaseSucceeded, 0
	if f.failed {
		wantPhase, wantExit = contract.PhaseFailed, failedExitCode
	}
	if snap.Run.Phase != wantPhase || snap.Run.Deleted {
		t.Errorf("run phase %s (deleted %v), want %s", snap.Run.Phase, snap.Run.Deleted, wantPhase)
	}
	if string(snap.Run.Runner) != f.runner {
		t.Errorf("runner %s, want %s", snap.Run.Runner, f.runner)
	}
	if len(snap.Attempts) != 1 {
		t.Fatalf("%d attempts, want 1", len(snap.Attempts))
	}
	a := snap.Attempts[0]
	if a.Container.State != contract.ContainerTerminated || a.Container.ExitCode == nil || *a.Container.ExitCode != wantExit {
		t.Errorf("container %+v, want terminated with exit code %d", a.Container, wantExit)
	}
	data, err := fixtures.Read(f.file)
	if err != nil {
		t.Fatal(err)
	}
	lines := int64(len(scenario.LogLines(string(data))))
	if a.Coverage.State != contract.CoverageEnded || a.Coverage.Lines != lines || a.Coverage.Gaps != 0 || a.Coverage.TruncatedLines != 0 {
		t.Errorf("coverage %+v, want ended, %d lines, no gap, no truncated line", a.Coverage, lines)
	}
	checkEventsOrdered(t, snap, a.ID)
}

// checkEventsOrdered checks the snapshot's events: strictly increasing
// seqs up to lastSeq, unique IDs, all of the attempt.
func checkEventsOrdered(t *testing.T, snap contract.Snapshot, attemptID string) {
	t.Helper()
	ids := map[string]bool{}
	var last int64
	for _, ev := range snap.Events {
		if ev.Seq <= last || ev.Seq > snap.Stream.LastSeq {
			t.Errorf("event %s: seq %d after %d, lastSeq %d", ev.ID, ev.Seq, last, snap.Stream.LastSeq)
		}
		last = ev.Seq
		if ids[ev.ID] {
			t.Errorf("event ID %s repeated", ev.ID)
		}
		ids[ev.ID] = true
		if ev.Attempt != attemptID {
			t.Errorf("event %s is of attempt %s, want %s", ev.ID, ev.Attempt, attemptID)
		}
		if ev.Source.LogTime == nil {
			t.Errorf("event %s has no log time", ev.ID)
		}
	}
}

// checkFixtureStream opens the stream from the snapshot: it holds the end
// of the run and nothing else, and a client that reconnects after it is
// told to stay away.
func checkFixtureStream(st *stack, key contract.RunKey, snap contract.Snapshot) {
	t := st.t
	t.Helper()
	c := st.events(key, streamOptions{after: cursor(snap)})
	if c.Status != http.StatusOK || c.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream: status %d, content type %q", c.Status, c.Header.Get("Content-Type"))
	}
	frames := c.drain()
	if len(frames) == 0 || frames[len(frames)-1].Event != contract.SSEEnd {
		t.Fatalf("the stream did not end with an end event: %v", summary(frames))
	}
	for _, f := range frames[:len(frames)-1] {
		if f.Event != contract.SSELive {
			t.Errorf("unexpected %s frame before the end", f.Event)
		}
	}
	end := frames[len(frames)-1]
	if got := end.end(t); got.Reason != contract.EndRunFinished || got.Seq != snap.Stream.LastSeq+1 {
		t.Errorf("end %+v, want run_finished at seq %d", got, snap.Stream.LastSeq+1)
	}
	for name, opts := range map[string]streamOptions{
		"after the end":       {after: end.ID},
		"Last-Event-ID":       {lastEventID: end.ID},
		"Last-Event-ID first": {after: cursor(snap), lastEventID: end.ID},
	} {
		if again := st.events(key, opts); again.Status != http.StatusNoContent {
			t.Errorf("reconnect %s: status %d, want 204", name, again.Status)
		}
	}
}

func TestFailedFixturesShowTheFailure(t *testing.T) {
	st := newStack(t)
	failed := map[string]int{}
	for _, f := range fixtureRuns(t, st.world) {
		if !f.failed {
			continue
		}
		failed[f.runner]++
		t.Run(f.runner+"/"+f.scenario, func(t *testing.T) {
			snap := st.with(t).snapshot(f.run.Key)
			a := snap.Attempts[0]
			if snap.Run.Phase != contract.PhaseFailed || *a.Container.ExitCode != failedExitCode || a.Container.Reason != "Error" {
				t.Errorf("run %s, container %+v: want failed, exit code %d, reason Error", snap.Run.Phase, a.Container, failedExitCode)
			}
			if a.Coverage.State != contract.CoverageEnded {
				t.Errorf("a failed run's log is read to its end: coverage %+v", a.Coverage)
			}
			// Failing is a state of the Task and the container: the log is
			// shown as it is, not replaced by an error of Lucarne's.
			if len(snap.Events) == 0 {
				t.Error("a failed run shows no event")
			}
		})
	}
	for _, runner := range []string{"claude-code", "codex", "opencode"} {
		if failed[runner] == 0 {
			t.Errorf("no failed %s fixture: the failure path is untested for it", runner)
		}
	}
}

func TestUnknownRunnerKeepsItsLinesRaw(t *testing.T) {
	st := newStack(t)
	snap := st.snapshot(st.world.Run(scenario.UnknownRunner).Key)
	if len(snap.Events) == 0 {
		t.Fatal("no event")
	}
	for _, ev := range snap.Events {
		if ev.Kind != contract.KindRaw {
			t.Errorf("event %s is a %s: no adapter knows this runner, its lines stay raw", ev.ID, ev.Kind)
		}
	}
	if snap.Attempts[0].Runner.Kind != contract.RunnerUnknown {
		t.Errorf("attempt runner %s, want unknown", snap.Attempts[0].Runner.Kind)
	}
}
