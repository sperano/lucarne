// Package adaptertest holds what the runner adapters' tests share: running
// lines through a Normalizer, checking events against the schema, and the
// golden tests over a runner's fixtures.
package adaptertest

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/privacy"
)

// goldenDir holds a runner package's golden files.
const goldenDir = "testdata"

// goldenMode is the permission of a rewritten golden file.
const goldenMode = 0o644

// Normalize runs lines through a Normalizer of attempt with runner, numbers
// the events as a stream would and checks each against the schema.
func Normalize(t testing.TB, attempt string, runner adapter.Runner, lines ...string) []contract.Event {
	t.Helper()
	var first []byte
	if len(lines) > 0 {
		first = []byte(lines[0])
	}
	n := adapter.New(attempt, contract.LogOrigin(first), runner)
	var events []contract.Event
	for i, line := range lines {
		events = append(events, n.Line(adapter.Line{Number: int64(i + 1), Text: []byte(line), Bytes: int64(len(line))})...)
	}
	events = append(events, n.End()...)
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	Validate(t, events)
	return events
}

// Validate checks every event against the schema and that no two share an
// ID.
func Validate(t testing.TB, events []contract.Event) {
	t.Helper()
	ids := map[string]bool{}
	for _, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal %s: %v", ev.ID, err)
		}
		if err := contract.Validate(contract.DefEvent, data); err != nil {
			t.Errorf("event %s: %v\n%s", ev.ID, err, data)
		}
		if ids[ev.ID] {
			t.Errorf("event ID %s is not unique", ev.ID)
		}
		ids[ev.ID] = true
	}
}

// Only returns the single event of kind, failing otherwise.
func Only(t testing.TB, events []contract.Event, kind contract.EventKind) contract.Event {
	t.Helper()
	found := OfKind(events, kind)
	if len(found) != 1 {
		t.Fatalf("got %d %s events, want 1: %s", len(found), kind, Dump(events))
	}
	return found[0]
}

// OfKind returns the events of kind, in order.
func OfKind(events []contract.Event, kind contract.EventKind) []contract.Event {
	var found []contract.Event
	for _, ev := range events {
		if ev.Kind == kind {
			found = append(found, ev)
		}
	}
	return found
}

// Dump is events as indented JSON, for failure messages.
func Dump(events []contract.Event) string {
	data, _ := json.MarshalIndent(events, "", "  ")
	return string(data)
}

// RunnerFixtures returns the manifest's fixtures of runner, failing when it
// lists none.
func RunnerFixtures(t testing.TB, runner string) []fixtures.Fixture {
	t.Helper()
	m, err := fixtures.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	var found []fixtures.Fixture
	for _, f := range m.Fixtures {
		if f.Runner == runner {
			found = append(found, f)
		}
	}
	if len(found) == 0 {
		t.Fatalf("the manifest lists no %s fixture", runner)
	}
	return found
}

// FixtureLines returns the lines of a fixture file.
func FixtureLines(t testing.TB, file string) []string {
	t.Helper()
	data, err := fixtures.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// MatchGolden normalizes every fixture of runner with a fresh adapter from
// newRunner and compares the events with testdata/<scenario>.golden.json,
// rewriting it first when update is set. Every line that is not blank must
// produce an event, and the output must hold no private data.
func MatchGolden(t *testing.T, attempt, runner string, newRunner func() adapter.Runner, update bool) {
	t.Helper()
	for _, f := range RunnerFixtures(t, runner) {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			lines := FixtureLines(t, f.File)
			events := Normalize(t, attempt, newRunner(), lines...)
			CheckEveryLineShown(t, lines, events)
			got, err := json.MarshalIndent(events, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			for _, finding := range privacy.Scan(string(got)) {
				t.Errorf("golden output: %v", finding)
			}
			compareGolden(t, strings.TrimSuffix(path.Base(f.File), ".log"), got, update)
		})
	}
}

func compareGolden(t *testing.T, scenario string, got []byte, update bool) {
	t.Helper()
	golden := filepath.Join(goldenDir, scenario+".golden.json")
	if update {
		if err := os.WriteFile(golden, got, goldenMode); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s (go test -update rewrites it):\n%s", golden, got)
	}
}

// outputsLine matches the lines of a Kelos outputs block after its start.
var outputsLine = regexp.MustCompile(`^([a-z][a-z0-9-]*:( .*)?|---KELOS_OUTPUTS_END---)$`)

// CheckEveryLineShown fails when a line that is not blank produced no
// event, unless it belongs to an outputs block shown as its start line's
// event.
func CheckEveryLineShown(t testing.TB, lines []string, events []contract.Event) {
	t.Helper()
	shown := map[int64]contract.Event{}
	for _, ev := range events {
		shown[ev.Source.Line] = ev
	}
	inBlock := false
	for i, line := range lines {
		number := int64(i + 1)
		ev, ok := shown[number]
		switch {
		case ok:
			inBlock = ev.Kind == contract.KindLifecycle && ev.Lifecycle.Phase == contract.LifecycleKelosOutputs
		case line == "":
		case inBlock && outputsLine.MatchString(line):
		default:
			t.Errorf("line %d produced no event: %q", number, line)
		}
	}
}

// FuzzSeeds adds every line of runner's fixtures to f's corpus.
func FuzzSeeds(f *testing.F, runner string) {
	m, err := fixtures.LoadManifest()
	if err != nil {
		f.Fatal(err)
	}
	for _, fx := range m.Fixtures {
		if fx.Runner != runner {
			continue
		}
		data, err := fixtures.Read(fx.File)
		if err != nil {
			f.Fatal(err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			f.Add(line)
		}
	}
}
