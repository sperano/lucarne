package contract

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

var exampleRun = RunKey{Cluster: "example", Namespace: "agents", TaskUID: "3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"}

var exampleAttempt = AttemptKey{PodUID: "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f", Container: "claude-code", Restart: 2}

func TestRunKeyValidate(t *testing.T) {
	if err := exampleRun.Validate(); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	bad := []RunKey{
		{Cluster: "", Namespace: "agents", TaskUID: exampleRun.TaskUID},
		{Cluster: "Example", Namespace: "agents", TaskUID: exampleRun.TaskUID},
		{Cluster: "example", Namespace: "agents-", TaskUID: exampleRun.TaskUID},
		{Cluster: "example", Namespace: "agents", TaskUID: strings.ToUpper(exampleRun.TaskUID)},
		{Cluster: "example", Namespace: "agents", TaskUID: "example-claude-42"},
	}
	for _, k := range bad {
		if err := k.Validate(); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%+v: err = %v, want ErrInvalidKey", k, err)
		}
	}
}

func TestRunKeyPaths(t *testing.T) {
	const tail = "example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b"
	if got, want := exampleRun.LinkPath(), "/runs/"+tail; got != want {
		t.Errorf("LinkPath = %q, want %q", got, want)
	}
	if got, want := exampleRun.SnapshotPath(), "/api/v1/runs/"+tail; got != want {
		t.Errorf("SnapshotPath = %q, want %q", got, want)
	}
	if got, want := exampleRun.EventsPath(), "/api/v1/runs/"+tail+"/events"; got != want {
		t.Errorf("EventsPath = %q, want %q", got, want)
	}
}

func TestAttemptIDRoundTrip(t *testing.T) {
	id := exampleAttempt.ID()
	if want := "7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.2"; id != want {
		t.Fatalf("ID = %q, want %q", id, want)
	}
	got, err := ParseAttemptID(id)
	if err != nil || got != exampleAttempt {
		t.Errorf("ParseAttemptID(%q) = %+v, %v", id, got, err)
	}
}

func TestParseAttemptIDRejectsNonCanonical(t *testing.T) {
	for _, id := range []string{
		"7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.02",
		"7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.-1",
		"7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code",
		"7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.Claude.0",
		"example-claude-42-x7k2p.claude-code.0",
	} {
		if _, err := ParseAttemptID(id); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("ParseAttemptID(%q) err = %v, want ErrInvalidKey", id, err)
		}
	}
}

// TestIdentifiersMatchSchemaPatterns keeps the Go formatters and the
// schema's patterns in step.
func TestIdentifiersMatchSchemaPatterns(t *testing.T) {
	defs := schemaDefs(t)
	pattern := func(def string) *regexp.Regexp {
		return regexp.MustCompile(defs[def]["pattern"].(string))
	}
	attemptID := exampleAttempt.ID()
	checks := map[string]string{
		"AttemptID": attemptID,
		"EventID":   EventID(attemptID, 42, 1),
		"UID":       exampleRun.TaskUID,
		"DNSLabel":  exampleRun.Namespace,
	}
	for def, value := range checks {
		if !pattern(def).MatchString(value) {
			t.Errorf("%q does not match $defs/%s", value, def)
		}
	}
	if gap := GapEventID(attemptID, 120); !pattern("EventID").MatchString(gap) || !strings.HasSuffix(gap, ":gap") {
		t.Errorf("gap id %q is not an EventID ending in :gap", gap)
	}
}

func TestTimestampIsWrittenInUTC(t *testing.T) {
	zone := time.FixedZone("UTC-7", -7*60*60)
	ts := Timestamp{time.Date(2026, 10, 25, 2, 0, 6, 120000000, zone)}
	got, err := json.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"2026-10-25T09:00:06.12Z"`; string(got) != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

func TestTimestampRejectsOffsets(t *testing.T) {
	var ts Timestamp
	if err := json.Unmarshal([]byte(`"2026-10-25T05:00:06-04:00"`), &ts); err == nil {
		t.Error("a time with an offset was accepted")
	}
	if err := json.Unmarshal([]byte(`"2026-10-25T09:00:06.120394512Z"`), &ts); err != nil {
		t.Errorf("a UTC time was rejected: %v", err)
	}
}
