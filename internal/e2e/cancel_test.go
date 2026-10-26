package e2e

import (
	"net/http"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/stream"
)

// TestDisconnectedClientReleasesItsRun proves, by what the hub does next,
// that a client that goes away mid-stream is no longer counted as
// watching: a hub that follows one run turns every other away while the
// run is watched, and drops it for another once the last client left.
func TestDisconnectedClientReleasesItsRun(t *testing.T) {
	st := newStack(t, func(c *stream.Config) { c.MaxRuns = 1 })
	first := newLive(t, st)
	second := st.events(first.run.Key, streamOptions{after: cursor(st.snapshot(first.run.Key))})
	second.untilLive()

	other, err := st.world.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	openable := func() bool { return st.get(other.Key.SnapshotPath()).Status == http.StatusOK }
	st.apiError(st.get(other.Key.SnapshotPath()), http.StatusServiceUnavailable, contract.ErrorUnavailable)

	// One of the two clients leaves, in the middle of a stream that goes
	// on: the run is still watched.
	first.write(1)
	first.events.transcript(1)
	first.events.Close()
	second.transcript(1)
	st.apiError(st.get(other.Key.SnapshotPath()), http.StatusServiceUnavailable, contract.ErrorUnavailable)

	// The last one leaves: the slot is free, the run it watched is dropped.
	second.Close()
	eventually(t, "the disconnected clients' run to be released", openable)
	if st.snapshot(other.Key).Run.Phase != contract.PhaseRunning {
		t.Error("the run that took the slot is not served")
	}
}
