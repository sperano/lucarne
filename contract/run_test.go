package contract

import (
	"encoding/json"
	"testing"
)

// TestSnapshotWithoutAttemptsValidates covers a snapshot built in Go for a
// pending run: its nil slices must encode as the arrays the schema requires.
func TestSnapshotWithoutAttemptsValidates(t *testing.T) {
	snapshot := Snapshot{
		Schema: SchemaVersion,
		Run:    Run{Key: exampleRun, TaskName: "example-opencode-44", Phase: PhasePending},
		Stream: Stream{Epoch: "k3v9q2"},
		Limits: DefaultLimits(),
	}
	for _, s := range []any{snapshot, &snapshot} {
		encoded, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(DefSnapshot, encoded); err != nil {
			t.Errorf("%T: %v\n%s", s, err, encoded)
		}
	}
}
