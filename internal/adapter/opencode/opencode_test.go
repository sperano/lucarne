package opencode

import (
	"flag"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
)

var update = flag.Bool("update", false, "rewrite the golden files from the adapter's output")

// testAttempt is the attempt every test log belongs to.
const testAttempt = "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.opencode.0"

// runnerName is the runner the fixtures under test belong to.
const runnerName = "opencode"

func normalize(t *testing.T, lines ...string) []contract.Event {
	t.Helper()
	return adaptertest.Normalize(t, testAttempt, New(), lines...)
}

func only(t *testing.T, events []contract.Event, kind contract.EventKind) contract.Event {
	t.Helper()
	return adaptertest.Only(t, events, kind)
}

func TestFixturesMatchGolden(t *testing.T) {
	t.Parallel()
	adaptertest.MatchGolden(t, testAttempt, runnerName, func() adapter.Runner { return New() }, *update)
}

func TestLineTypesAndRawReasons(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want contract.RawReason
	}{
		{"text", "Warning: Unrecognized provider prefix 'example'", contract.RawText},
		{"cut object", `{"type":"text","timestamp":1,"part":{"te`, contract.RawMalformedJSON},
		{"array", `[1,2]`, contract.RawText},
		{"unknown type", `{"type":"example_future_event"}`, contract.RawUnknownType},
		{"no type", `{"example":1}`, contract.RawUnknownType},
		{"type not a string", `{"type":7}`, contract.RawUnknownType},
		{"text without part", `{"type":"text"}`, contract.RawUnsupported},
		{"text without text", `{"type":"text","part":{"type":"text"}}`, contract.RawUnsupported},
		{"text of another part type", `{"type":"text","part":{"type":"tool","text":"a"}}`, contract.RawUnsupported},
		{"part not an object", `{"type":"step_start","part":7}`, contract.RawUnsupported},
		{"step_start of another part", `{"type":"step_start","part":{"type":"step-finish"}}`, contract.RawUnsupported},
		{"step_finish of another part", `{"type":"step_finish","part":{"type":"step-start"}}`, contract.RawUnsupported},
		{"tool without call ID", `{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"status":"completed"}}}`, contract.RawUnsupported},
		{"tool without state", `{"type":"tool_use","part":{"type":"tool","callID":"c","tool":"bash"}}`, contract.RawUnsupported},
		{"tool without status", `{"type":"tool_use","part":{"type":"tool","callID":"c","tool":"bash","state":{}}}`, contract.RawUnsupported},
		{"error without error", `{"type":"error"}`, contract.RawUnsupported},
		{"null error", `{"type":"error","error":null}`, contract.RawUnsupported},
		{"synthetic not a boolean", `{"type":"text","part":{"type":"text","text":"a","synthetic":"yes"}}`, contract.RawUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			raw := only(t, normalize(t, c.line), contract.KindRaw)
			if raw.Raw.Reason != c.want || raw.Raw.Text != c.line || raw.Raw.Bytes != int64(len(c.line)) {
				t.Errorf("raw = %+v, want reason %s holding the line", raw.Raw, c.want)
			}
		})
	}
}

func TestRawTextIsBounded(t *testing.T) {
	t.Parallel()
	line := `{"type":"example_future_event","data":"` + strings.Repeat("é", contract.MaxRawBytes) + `"}`
	raw := only(t, normalize(t, line), contract.KindRaw)
	if len(raw.Raw.Text) > contract.MaxRawBytes || raw.Raw.Bytes != int64(len(line)) {
		t.Errorf("raw text is %d bytes for a %d-byte line, bytes = %d", len(raw.Raw.Text), len(line), raw.Raw.Bytes)
	}
	if len(raw.Truncated) != 1 || raw.Truncated[0] != "text" {
		t.Errorf("truncated = %v, want [text]", raw.Truncated)
	}
}

func TestRunnerTimeIsTheLineTimestamp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		timestamp string
		want      string
	}{
		{"milliseconds", `1792915204400`, "2026-10-25T08:00:04.400Z"},
		{"zero", `0`, "1970-01-01T00:00:00.000Z"},
		{"negative", `-1`, ""},
		{"past year 9999", `253402300800000`, ""},
		{"fraction", `1792915204400.5`, ""},
		{"string", `"1792915204400"`, ""},
		{"null", `null`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			line := `{"type":"step_start","timestamp":` + c.timestamp + `,"part":{"type":"step-start"}}`
			ev := only(t, normalize(t, line), contract.KindLifecycle)
			got := ""
			if ev.Source.RunnerTime != nil {
				got = ev.Source.RunnerTime.Format("2006-01-02T15:04:05.000Z07:00")
			}
			if got != c.want {
				t.Errorf("runnerTime = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSourceTypeIsTheLineType(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"step_finish","part":{"type":"step-finish","reason":"stop","cost":0}}`,
		`{"type":"example_future_event"}`,
	)
	for _, ev := range events[:2] {
		if ev.Source.Type != typeStepFinish {
			t.Errorf("source type = %q, want %q", ev.Source.Type, typeStepFinish)
		}
	}
	if events[2].Source.Type != "example_future_event" {
		t.Errorf("source type of an unknown line = %q", events[2].Source.Type)
	}
}

func FuzzLine(f *testing.F) {
	adaptertest.FuzzSeeds(f, runnerName)
	f.Fuzz(func(t *testing.T, line string) {
		events := normalize(t, line, line)
		if line != "" && len(events) == 0 {
			t.Errorf("line %q produced no event", line)
		}
	})
}
