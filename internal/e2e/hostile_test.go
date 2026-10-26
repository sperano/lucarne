package e2e

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/e2e/scenario"
)

// zeroUID is a well-formed UID no Task has.
const zeroUID = "00000000-0000-4000-8000-000000000000"

// escape is a request for a run outside the server's reach, and what the
// server must answer.
type escape struct {
	name, path string
	// status and code are the answer wanted; status 0 accepts a 400 or a
	// 404, whatever the path means once cleaned.
	status int
	code   contract.ErrorCode
}

func escapes(w *scenario.World) []escape {
	private := w.Run(scenario.PrivateRun).Key
	legit := w.Run(scenario.Hostile).Key
	api := contract.APIPrefix + "/runs/"
	return []escape{
		{"private namespace", private.SnapshotPath(), http.StatusNotFound, contract.ErrorRunNotFound},
		{"private namespace stream", private.EventsPath(), http.StatusNotFound, contract.ErrorRunNotFound},
		{"private UID in the allowed namespace", api + scenario.Cluster + "/" + scenario.Namespace + "/" + private.TaskUID, http.StatusNotFound, contract.ErrorRunNotFound},
		{"private UID in the allowed namespace, stream", api + scenario.Cluster + "/" + scenario.Namespace + "/" + private.TaskUID + "/events", http.StatusNotFound, contract.ErrorRunNotFound},
		{"another cluster", api + "other/" + legit.Namespace + "/" + legit.TaskUID, http.StatusNotFound, contract.ErrorRunNotFound},
		{"another cluster, private namespace", api + "other/" + scenario.PrivateNamespace + "/" + private.TaskUID, http.StatusNotFound, contract.ErrorRunNotFound},
		{"no such UID", api + scenario.Cluster + "/" + scenario.Namespace + "/" + zeroUID, http.StatusNotFound, contract.ErrorRunNotFound},
		{"task name for a UID", api + scenario.Cluster + "/" + scenario.Namespace + "/example-hostile", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"uppercase UID", api + scenario.Cluster + "/" + scenario.Namespace + "/" + strings.ToUpper(legit.TaskUID), http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"NUL in the UID", api + scenario.Cluster + "/" + scenario.Namespace + "/%00", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"trailing slash", legit.SnapshotPath() + "/", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"extra segment", legit.EventsPath() + "/more", http.StatusBadRequest, contract.ErrorInvalidRunKey},
		{"dot segments", api + scenario.Cluster + "/" + scenario.Namespace + "/../" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"dot segments after the UID", legit.SnapshotPath() + "/../../../" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"encoded dot segments", api + scenario.Cluster + "/" + scenario.Namespace + "/%2e%2e/" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"double-encoded dot segments", api + scenario.Cluster + "/" + scenario.Namespace + "/%252e%252e/" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"encoded slash in the namespace", api + scenario.Cluster + "/" + scenario.Namespace + "%2F" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"encoded slash in the cluster", api + scenario.Cluster + "%2Fx/" + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
		{"encoded slash before the UID", api + scenario.Cluster + "/" + scenario.PrivateNamespace + "%2F" + private.TaskUID, 0, ""},
		{"encoded namespace", api + scenario.Cluster + "/%70rivate/" + private.TaskUID, 0, ""},
		{"empty namespace", api + scenario.Cluster + "//" + private.TaskUID, 0, ""},
		{"backslashes", api + scenario.Cluster + `/..%5c` + scenario.PrivateNamespace + "/" + private.TaskUID, 0, ""},
	}
}

func TestNamespaceEscapesAreRefused(t *testing.T) {
	st := newStack(t)
	for _, e := range escapes(st.world) {
		t.Run(e.name, func(t *testing.T) {
			st := st.with(t)
			r := st.get(e.path)
			switch {
			case e.status != 0:
				st.apiError(r, e.status, e.code)
			case r.Status != http.StatusBadRequest && r.Status != http.StatusNotFound:
				t.Fatalf("GET %s: status %d, want 400 or 404: %s", e.path, r.Status, truncateForLog(r.Body))
			}
			if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				if err := contract.Validate(contract.DefErrorResponse, r.Body); err != nil {
					t.Errorf("GET %s: %v", e.path, err)
				}
			}
		})
	}
	st.assertNeverSent(scenario.PrivateMarker, scenario.ForeignMarker)
	if st.world.Cluster.Namespaces()[scenario.PrivateNamespace] {
		t.Errorf("the source called the API in %s, which is not allowlisted", scenario.PrivateNamespace)
	}
	for _, o := range st.world.Cluster.LogOpens() {
		if o.Namespace == scenario.PrivateNamespace {
			t.Errorf("a log was opened in %s: %+v", o.Namespace, o)
		}
	}
}

func TestExtraParametersCannotSelectWhatToRead(t *testing.T) {
	st := newStack(t)
	legit := st.world.Run(scenario.ForeignPod).Key
	private := st.world.Run(scenario.PrivateRun).Key
	query := "?namespace=" + scenario.PrivateNamespace + "&cluster=other&taskUID=" + private.TaskUID +
		"&pod=example-private-abcde&container=git-clone&attempt=" + private.TaskUID
	snap := st.snapshot(legit)
	r := st.get(legit.SnapshotPath() + query)
	if r.Status != http.StatusOK {
		t.Fatalf("status %d: %s", r.Status, truncateForLog(r.Body))
	}
	if !bytes.Contains(r.Body, []byte(snap.Run.Key.TaskUID)) || bytes.Contains(r.Body, []byte(private.TaskUID)) {
		t.Error("the snapshot is not the run the path names")
	}
	frames := st.events(legit, streamOptions{after: cursor(snap)}).drain()
	if len(frames) == 0 {
		t.Error("no stream")
	}
	st.assertNeverSent(scenario.PrivateMarker, scenario.ForeignMarker, private.TaskUID)
}

func TestOnlyTheRunsOwnPodsAreRead(t *testing.T) {
	st := newStack(t)
	key := st.world.Run(scenario.ForeignPod).Key
	snap := st.snapshot(key)
	// The foreign pod carries the run's label but is not owned through its
	// Job: the run has its own pod's attempt, and nothing of the other.
	if len(snap.Attempts) != 1 {
		t.Fatalf("%d attempts, want the one of the run's own pod", len(snap.Attempts))
	}
	own := snap.Attempts[0].PodName
	var texts []string
	for _, ev := range snap.Events {
		if ev.Message != nil {
			texts = append(texts, ev.Message.Text)
		}
	}
	if len(texts) != 1 || texts[0] != "The run's own line." {
		t.Errorf("messages %q, want the run's own line only", texts)
	}
	st.get(key.EventsPath() + "?after=" + cursor(snap))
	prefix := strings.TrimSuffix(own, "-abcde")
	opened := false
	for _, o := range st.world.Cluster.LogOpens() {
		switch {
		case o.Pod == own:
			opened = true
		case strings.HasPrefix(o.Pod, prefix+"-"):
			t.Errorf("the log of %s/%s was opened: it is labelled for the run but not owned by its Job", o.Namespace, o.Pod)
		}
	}
	if !opened {
		t.Error("the run's own pod was never read: the check is vacuous")
	}
	st.assertNeverSent(scenario.ForeignMarker, scenario.PrivateMarker)
}

func TestHostileTranscriptIsCarriedAsData(t *testing.T) {
	st := newStack(t)
	key := st.world.Run(scenario.Hostile).Key
	snap, r := st.snapshotResponse(key)
	checkJSONHeaders(t, r)
	checkHostileBody(t, r.Body)

	if c := snap.Attempts[0].Coverage; int(c.Lines) != len(snap.Events) || c.State != contract.CoverageEnded {
		t.Errorf("coverage %+v for %d events: every hostile line is shown as an event", c, len(snap.Events))
	}
	checkHostileMarkdown(t, snap.Events)
	raws := 0
	for _, ev := range snap.Events {
		if ev.Raw == nil {
			continue
		}
		raws++
		// Bytes is the line's length as the log held it: the text can be
		// longer when invalid UTF-8 was replaced by U+FFFD (3 bytes for
		// 1 or more), so only the bounds are checked.
		if len(ev.Raw.Text) > contract.MaxRawBytes || !utf8.ValidString(ev.Raw.Text) || ev.Raw.Bytes <= 0 {
			t.Errorf("raw event %s: %d bytes of text for a line of %d, valid UTF-8 %v", ev.ID, len(ev.Raw.Text), ev.Raw.Bytes, utf8.ValidString(ev.Raw.Text))
		}
	}
	if raws < len(scenario.HostileRaw)/2 {
		t.Errorf("%d raw events for %d hostile lines", raws, len(scenario.HostileRaw))
	}
	// Lines carried verbatim, control characters and markup included:
	// escaping them is the UI's job.
	for _, line := range []string{scenario.HostileRaw[4], scenario.HostileRaw[7], scenario.HostileRaw[3]} {
		if !hasRawText(snap.Events, line) {
			t.Errorf("no raw event carries %q verbatim", line)
		}
	}
	st.assertNeverSent(scenario.PrivateMarker, scenario.ForeignMarker)
}

// checkHostileBody checks what a JSON document that carries markup must
// be: valid UTF-8, with no raw angle bracket, so that it is no HTML even
// if served as such.
func checkHostileBody(t *testing.T, body []byte) {
	t.Helper()
	if !utf8.Valid(body) {
		t.Error("the response is not valid UTF-8")
	}
	if bytes.ContainsAny(body, "<>") {
		t.Error("the response holds a raw angle bracket: markup must be JSON-escaped")
	}
}

// checkHostileMarkdown checks that the hostile message is the text the
// runner wrote, link by link.
func checkHostileMarkdown(t *testing.T, events []contract.Event) {
	t.Helper()
	for _, ev := range events {
		if ev.Message == nil || !strings.HasPrefix(ev.Message.Text, "Hostile Markdown follows") {
			continue
		}
		at := 0
		for _, md := range scenario.HostileMarkdown {
			i := strings.Index(ev.Message.Text[at:], md)
			if i < 0 {
				t.Errorf("the message lost %q or reordered it", md)
				continue
			}
			at += i + len(md)
		}
		return
	}
	t.Error("no hostile message")
}

func hasRawText(events []contract.Event, text string) bool {
	for _, ev := range events {
		if ev.Raw != nil && ev.Raw.Text == text {
			return true
		}
	}
	return false
}

func TestHostileLinesOverTheLiveStream(t *testing.T) {
	st := newStack(t)
	l := newLive(t, st)
	lines := []string{scenario.ClaudeText(1, "Hostile Markdown follows.\n\n"+strings.Join(scenario.HostileMarkdown, "\n\n"))}
	lines = append(lines, scenario.HostileRaw...)
	l.run.Pod.Write(lines...)
	frames, ok := l.events.until(func(f frame) bool {
		return len(ofEvent(l.events.Seen, contract.SSETranscript)) == len(lines)
	})
	if !ok {
		t.Fatalf("the stream ended: %v", summary(frames))
	}
	for _, f := range ofEvent(frames, contract.SSETranscript) {
		checkHostileBody(t, []byte(f.Data))
	}
	checkHostileMarkdown(t, transcriptOf(t, frames))
	snap := st.snapshot(l.run.Key)
	if len(snap.Events) != len(lines) {
		t.Errorf("%d events for %d lines", len(snap.Events), len(lines))
	}
}
