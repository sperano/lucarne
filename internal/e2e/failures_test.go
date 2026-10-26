package e2e

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

// sourceFailure is an error of the Kubernetes API injected for a verb on
// a resource. Its text names what an error can carry and a response must
// not.
type sourceFailure struct {
	name, verb, resource string
	err                  func(text string) error
	// refused is a failure of the credentials, answered with its own
	// message.
	refused bool
}

var tasksResource = schema.GroupResource{Group: "kelos.dev", Resource: "tasks"}

var sourceFailures = []sourceFailure{
	{name: "tasks unavailable", verb: "list", resource: "tasks",
		err: func(text string) error { return apierrors.NewServiceUnavailable(text) }},
	{name: "tasks forbidden", verb: "list", resource: "tasks", refused: true,
		err: func(text string) error { return apierrors.NewForbidden(tasksResource, "", errors.New(text)) }},
	{name: "tasks unauthorized", verb: "list", resource: "tasks", refused: true,
		err: func(text string) error { return apierrors.NewUnauthorized(text) }},
	{name: "pods internal error", verb: "list", resource: "pods",
		err: func(text string) error { return apierrors.NewInternalError(errors.New(text)) }},
	{name: "job forbidden", verb: "get", resource: "jobs", refused: true,
		err: func(text string) error {
			return apierrors.NewForbidden(schema.GroupResource{Group: "batch", Resource: "jobs"}, "job", errors.New(text))
		}},
}

func TestSourceFailuresAnswerFixedMessages(t *testing.T) {
	messages := map[bool]map[string]bool{false: {}, true: {}}
	for _, f := range sourceFailures {
		t.Run(f.name, func(t *testing.T) {
			// The fake's reactors are not safe to add while a run is being
			// followed: this test's hub reads nothing before the failure is
			// injected, and the earlier tests' hubs are closed.
			run, err := sharedWorld(t).AddLive()
			if err != nil {
				t.Fatal(err)
			}
			text := f.name + " at " + internalHost
			restore := sharedWorld(t).Cluster.Fail(f.verb, f.resource, f.err(text))
			st := newStack(t)

			for _, path := range []string{run.Key.SnapshotPath(), run.Key.EventsPath()} {
				r := st.get(path)
				body := st.apiError(r, http.StatusServiceUnavailable, contract.ErrorUnavailable)
				checkJSONHeaders(t, r)
				if body.Message == "" {
					t.Errorf("GET %s: message %q must be a fixed one", path, body.Message)
				}
				messages[f.refused][body.Message] = true
			}
			st.assertNeverSent(internalHost, "s3cr3t")
			if !strings.Contains(string(st.logs.bytes()), "s3cr3t") {
				t.Error("the failure was not logged: it is how the operator diagnoses the source")
			}

			// Nothing is cached of a run that could not be opened: it opens
			// once the API answers.
			restore()
			snap := st.snapshot(run.Key)
			if snap.Run.Phase != contract.PhaseRunning {
				t.Errorf("after the failure, phase %s", snap.Run.Phase)
			}
		})
	}
	if len(messages[false]) != 1 || len(messages[true]) != 1 {
		t.Errorf("messages %v: want one for an API that did not answer and one for credentials it refused", messages)
	}
	for m := range messages[false] {
		if messages[true][m] {
			t.Errorf("refused credentials and an unreachable API share the message %q", m)
		}
	}
}

func TestRunWhosePodIsGoneListsNoAttempt(t *testing.T) {
	st := newStack(t)
	key := st.world.Run(scenario.PodDeleted).Key
	snap := st.snapshot(key)
	// Nothing says a deleted pod existed: no attempt is listed, and the
	// run, finished, has no log to wait for.
	if len(snap.Attempts) != 0 || len(snap.Events) != 0 {
		t.Errorf("%d attempts, %d events: a deleted pod is not listed and its lines are lost", len(snap.Attempts), len(snap.Events))
	}
	if snap.Run.Phase != contract.PhaseSucceeded || snap.Run.Deleted {
		t.Errorf("run %s (deleted %v): the Task is still there", snap.Run.Phase, snap.Run.Deleted)
	}
	frames := st.events(key, streamOptions{after: cursor(snap)}).drain()
	if len(frames) == 0 || frames[len(frames)-1].Event != contract.SSEEnd {
		t.Fatalf("stream %v, want it to end", summary(frames))
	}
	if reason := frames[len(frames)-1].end(t).Reason; reason != contract.EndRunFinished {
		t.Errorf("end %s, want run_finished", reason)
	}
}
