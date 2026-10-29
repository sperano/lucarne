package e2e

import (
	"net/http"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

// fabricatedPrompt is the prompt of every Task of the fake cluster, which
// the source drops when it decodes a Task.
const fabricatedPrompt = "fabricated prompt"

// TestRunListOverTheCluster proves the run list holds the runs of the
// allowlisted namespace, newest first, and neither the private namespace's run nor any Task's prompt.
func TestRunListOverTheCluster(t *testing.T) {
	st := newStack(t)
	r := st.get(contract.RunsPath)
	if r.Status != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", contract.RunsPath, r.Status, truncateForLog(r.Body))
	}
	checkJSONHeaders(t, r)
	if err := contract.Validate(contract.DefRunList, r.Body); err != nil {
		t.Fatal(err)
	}
	var list contract.RunList
	mustUnmarshal(t, r.Body, &list)
	listed := map[contract.RunKey]bool{}
	for i, run := range list.Runs {
		listed[run.Key] = true
		if run.Key.Cluster != scenario.Cluster || run.Key.Namespace != scenario.Namespace {
			t.Errorf("listed %s, outside the allowlist", run.Key.LinkPath())
		}
		if i > 0 && run.Created != nil && list.Runs[i-1].Created != nil && run.Created.After(list.Runs[i-1].Created.Time) {
			t.Errorf("%s is listed after an older run", run.TaskName)
		}
	}
	for _, name := range []string{scenario.Hostile, scenario.Retried, scenario.LongSession} {
		if key := st.world.Run(name).Key; !listed[key] {
			t.Errorf("run %s is not listed", name)
		}
	}
	private := st.world.Run(scenario.PrivateRun).Key
	st.assertNeverSent(private.TaskUID, scenario.PrivateNamespace, fabricatedPrompt)
}
