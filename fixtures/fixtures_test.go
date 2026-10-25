package fixtures

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/sperano/lucarne/internal/privacy"
)

// knownScenarios is the vocabulary of Fixture.Scenarios.
var knownScenarios = []string{
	"startup_text", "setup_markers", "session_start", "assistant_text",
	"reasoning", "tool_success", "tool_failure", "edit", "usage", "cost",
	"retry", "rate_limit", "limit_hit", "session_failure", "malformed_json",
	"truncated_line", "non_json_line", "unknown_event", "unknown_item",
	"kelos_outputs", "todo", "mcp_tool", "compaction", "duplicate_keys",
	"parallel_tools", "subagent", "partial_message", "duplicate_line",
}

// requiredScenarios must each be covered by a runner's fixtures at the
// deployed version, or be documented in notObservable.
var requiredScenarios = []string{
	"startup_text", "session_start", "assistant_text", "reasoning",
	"tool_success", "tool_failure", "edit", "usage", "retry",
	"session_failure", "malformed_json", "truncated_line", "non_json_line",
	"unknown_event", "kelos_outputs",
}

func loadManifest(t *testing.T) Manifest {
	t.Helper()
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifestListsEveryFixtureOnce(t *testing.T) {
	t.Parallel()
	m := loadManifest(t)
	onDisk, err := fs.Glob(Files, "*/*/*.log")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, f := range m.Fixtures {
		listed = append(listed, f.File)
	}
	slices.Sort(listed)
	if !slices.Equal(listed, onDisk) {
		t.Errorf("manifest lists %v, files are %v", listed, onDisk)
	}
	if len(slices.Compact(slices.Clone(listed))) != len(listed) {
		t.Errorf("manifest lists a file twice: %v", listed)
	}
}

func TestFixturesAreProvenanceLabelled(t *testing.T) {
	t.Parallel()
	m := loadManifest(t)
	for _, f := range m.Fixtures {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			if want := path.Join(f.Runner, f.Version, path.Base(f.File)); f.File != want {
				t.Errorf("file %q is not under its runner and version, want %q", f.File, want)
			}
			if len(m.Basis[f.Runner]) == 0 {
				t.Errorf("runner %q has no basis", f.Runner)
			}
			switch f.Provenance {
			case ProvenanceSynthetic:
				if f.CapturedOn != "" {
					t.Error("a synthetic fixture has a capture date")
				}
			case ProvenanceCaptured:
				if f.CapturedOn == "" {
					t.Error("a captured fixture has no capture date")
				}
			default:
				t.Errorf("provenance %q is not %q or %q", f.Provenance, ProvenanceSynthetic, ProvenanceCaptured)
			}
			if strings.TrimSpace(f.Notes) == "" {
				t.Error("no notes")
			}
			for _, s := range f.Scenarios {
				if !slices.Contains(knownScenarios, s) {
					t.Errorf("unknown scenario %q", s)
				}
			}
		})
	}
}

func TestLineCountsMatchContent(t *testing.T) {
	t.Parallel()
	for _, f := range loadManifest(t).Fixtures {
		t.Run(f.File, func(t *testing.T) {
			t.Parallel()
			if got := countLines(t, f.File); got != f.Lines {
				t.Errorf("lines = %+v, manifest says %+v", got, f.Lines)
			}
		})
	}
}

func countLines(t *testing.T, file string) LineCounts {
	t.Helper()
	data, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
	content, ok := strings.CutSuffix(string(data), "\n")
	if !ok {
		t.Fatal("the file does not end with a newline")
	}
	var counts LineCounts
	for _, line := range strings.Split(content, "\n") {
		var object map[string]any
		switch {
		case line == "":
			counts.Blank++
		case json.Unmarshal([]byte(line), &object) == nil:
			counts.JSON++
		default:
			counts.Text++
		}
	}
	return counts
}

func TestDeployedVersionsAreCovered(t *testing.T) {
	t.Parallel()
	m := loadManifest(t)
	for runner, version := range m.Deployed.Runners {
		t.Run(runner, func(t *testing.T) {
			t.Parallel()
			covered := map[string]bool{}
			for _, f := range m.Fixtures {
				if f.Runner == runner && f.Version == version {
					for _, s := range f.Scenarios {
						covered[s] = true
					}
				}
			}
			for _, s := range requiredScenarios {
				_, documented := m.NotObservable[runner][s]
				if !covered[s] && !documented {
					t.Errorf("scenario %q is neither covered at %s nor documented as not observable", s, version)
				}
				if covered[s] && documented {
					t.Errorf("scenario %q is covered at %s and documented as not observable", s, version)
				}
			}
		})
	}
}

// readmeFile documents the fixtures; it is not embedded.
const readmeFile = "README.md"

func TestFixturesHoldNoPrivateData(t *testing.T) {
	t.Parallel()
	files := []string{ManifestFile}
	for _, f := range loadManifest(t).Fixtures {
		files = append(files, f.File)
	}
	for _, file := range files {
		data, err := Read(file)
		if err != nil {
			t.Fatal(err)
		}
		scanForPrivateData(t, file, data)
	}
	readme, err := os.ReadFile(readmeFile)
	if err != nil {
		t.Fatal(err)
	}
	scanForPrivateData(t, readmeFile, readme)
}

func scanForPrivateData(t *testing.T, file string, data []byte) {
	t.Helper()
	for _, finding := range privacy.Scan(string(data)) {
		t.Errorf("%s: %v", file, finding)
	}
}
