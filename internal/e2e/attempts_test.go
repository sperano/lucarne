package e2e

import (
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

func TestRetriedRunListsEachAttemptWithItsOwnEvents(t *testing.T) {
	st := newStack(t)
	snap := st.snapshot(st.world.Run(scenario.Retried).Key)
	if len(snap.Attempts) != 2 {
		t.Fatalf("%d attempts, want the failed pod's and the retry's", len(snap.Attempts))
	}
	wantExit := []int{failedExitCode, 0}
	wantText := []string{scenario.RetriedFirstText, scenario.RetriedSecondText}
	for i, a := range snap.Attempts {
		if a.Container.ExitCode == nil || *a.Container.ExitCode != wantExit[i] {
			t.Errorf("attempt %d container %+v, want exit code %d", i+1, a.Container, wantExit[i])
		}
		if a.Coverage.State != contract.CoverageEnded {
			t.Errorf("attempt %d coverage %+v, want its log read to its end", i+1, a.Coverage)
		}
		if got := messagesOf(snap, a.ID); len(got) != 1 || got[0] != wantText[i] {
			t.Errorf("attempt %d messages %q, want only %q", i+1, got, wantText[i])
		}
	}
	if snap.Attempts[0].Key.PodUID == snap.Attempts[1].Key.PodUID {
		t.Error("both attempts name the same pod")
	}
}

// messagesOf are the texts of the messages of attempt in snap.
func messagesOf(snap contract.Snapshot, attempt string) []string {
	var texts []string
	for _, ev := range snap.Events {
		if ev.Attempt == attempt && ev.Message != nil {
			texts = append(texts, ev.Message.Text)
		}
	}
	return texts
}
