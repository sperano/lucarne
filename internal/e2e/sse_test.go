package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/stream"
)

// documents maps an SSE event to the definition of its data.
var documents = map[string]string{
	contract.SSETranscript: contract.DefEvent,
	contract.SSEAttempt:    contract.DefAttemptUpdate,
	contract.SSERun:        contract.DefRunUpdate,
	contract.SSEEnd:        contract.DefEnd,
	contract.SSEReset:      contract.DefReset,
	contract.SSELive:       contract.DefLive,
}

// frame is one SSE message.
type frame struct {
	Event, ID, Data string
}

// event decodes the data of a transcript frame.
func (f frame) event(t *testing.T) contract.Event {
	t.Helper()
	var ev contract.Event
	mustUnmarshal(t, []byte(f.Data), &ev)
	return ev
}

// end decodes the data of an end frame.
func (f frame) end(t *testing.T) contract.End {
	t.Helper()
	var e contract.End
	mustUnmarshal(t, []byte(f.Data), &e)
	return e
}

// reset decodes the data of a reset frame.
func (f frame) reset(t *testing.T) contract.Reset {
	t.Helper()
	var r contract.Reset
	mustUnmarshal(t, []byte(f.Data), &r)
	return r
}

// sseClient reads a run's stream over HTTP, checking as a client must that
// every frame is valid and every seq follows the last.
type sseClient struct {
	t      *testing.T
	Status int
	Header http.Header
	// Body is what a response that is not a stream held.
	Body   []byte
	frames chan frame
	cancel context.CancelFunc
	// seqOnly skips the schema check.
	seqOnly bool
	epoch   string
	last    int64
	// Seen are all the frames read.
	Seen []frame
}

// streamOptions are how a stream is opened.
type streamOptions struct {
	after, lastEventID string
	// client reads the stream; nil means the stack's.
	client *http.Client
	// seqOnly skips the schema check of each frame, for a stream of
	// thousands that the schema tests elsewhere cover; the ids are still
	// checked.
	seqOnly bool
}

// events opens the run's stream.
func (s *stack) events(key contract.RunKey, o streamOptions) *sseClient {
	s.t.Helper()
	path := key.EventsPath()
	if o.after != "" {
		path += "?after=" + o.after
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.srv.URL+path, nil)
	if err != nil {
		cancel()
		s.t.Fatal(err)
	}
	if o.lastEventID != "" {
		req.Header.Set("Last-Event-ID", o.lastEventID)
	}
	client := o.client
	if client == nil {
		client = s.client
	}
	res, err := client.Do(req)
	if err != nil {
		cancel()
		s.t.Fatalf("GET %s: %v", path, err)
	}
	c := &sseClient{t: s.t, Status: res.StatusCode, Header: res.Header, frames: make(chan frame, 1), cancel: cancel, seqOnly: o.seqOnly}
	s.t.Cleanup(c.Close)
	c.start(o.after, o.lastEventID)
	if res.StatusCode != http.StatusOK {
		defer func() { _ = res.Body.Close() }()
		c.Body, _ = io.ReadAll(res.Body)
		close(c.frames)
		return c
	}
	go c.pump(res.Body)
	return c
}

// start sets the cursor the first numbered frame must follow.
func (c *sseClient) start(after, lastEventID string) {
	text := after
	if lastEventID != "" {
		text = lastEventID
	}
	if cur, ok := stream.ParseCursor(text); ok {
		c.epoch, c.last = cur.Epoch, cur.Seq
	}
}

// Close stops reading the stream.
func (c *sseClient) Close() { c.cancel() }

// pump parses the body into frames until it ends.
func (c *sseClient) pump(body io.ReadCloser) {
	defer close(c.frames)
	defer func() { _ = body.Close() }()
	r := bufio.NewReader(body)
	var f frame
	var started bool
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSuffix(line, "\n")
		field, value, _ := strings.Cut(line, ": ")
		switch field {
		case "":
			if started {
				c.frames <- f
				f, started = frame{}, false
			}
		case "event":
			f.Event, started = value, true
		case "id":
			f.ID, started = value, true
		case "data":
			f.Data, started = f.Data+value, true
		}
		// A line starting with ":" is a keepalive comment: ignored.
	}
}

// next is the next frame, false once the stream ended. It fails the test
// when the frame is invalid or out of order, or none comes in time.
func (c *sseClient) next() (frame, bool) {
	c.t.Helper()
	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()
	select {
	case f, ok := <-c.frames:
		if ok {
			c.check(f)
			c.Seen = append(c.Seen, f)
		}
		return f, ok
	case <-timer.C:
		c.t.Fatalf("no frame in %s; read so far: %v", waitTimeout, summary(c.Seen))
		return frame{}, false
	}
}

// check validates a frame against the schema and the seq rule: each id is
// in the epoch of the cursor and its seq is the last plus one.
func (c *sseClient) check(f frame) {
	c.t.Helper()
	def, ok := documents[f.Event]
	if !ok {
		c.t.Fatalf("unknown SSE event %q", f.Event)
	}
	if err := validateUnless(c.seqOnly, def, f.Data); err != nil {
		c.t.Fatalf("%s frame: %v\n%s", f.Event, err, truncateForLog([]byte(f.Data)))
	}
	if f.ID == "" {
		if f.Event != contract.SSELive && f.Event != contract.SSEReset {
			c.t.Errorf("%s frame has no id", f.Event)
		}
		return
	}
	cur, ok := stream.ParseCursor(f.ID)
	if !ok {
		c.t.Fatalf("%s frame: id %q is not a cursor", f.Event, f.ID)
	}
	if c.epoch == "" {
		c.epoch, c.last = cur.Epoch, cur.Seq-1
	}
	if cur.Epoch != c.epoch || cur.Seq != c.last+1 {
		c.t.Fatalf("%s frame: id %s follows %s:%d: an update was lost or repeated", f.Event, f.ID, c.epoch, c.last)
	}
	c.last = cur.Seq
}

// until reads frames up to and including the first one done accepts, and
// reports false when the stream ended before.
func (c *sseClient) until(done func(frame) bool) ([]frame, bool) {
	c.t.Helper()
	var read []frame
	for {
		f, ok := c.next()
		if !ok {
			return read, false
		}
		read = append(read, f)
		if done(f) {
			return read, true
		}
	}
}

// drain reads until the stream ends.
func (c *sseClient) drain() []frame {
	c.t.Helper()
	frames, _ := c.until(func(frame) bool { return false })
	return frames
}

// untilLive reads the replay and the live marker.
func (c *sseClient) untilLive() []frame {
	c.t.Helper()
	frames, ok := c.until(func(f frame) bool { return f.Event == contract.SSELive })
	if !ok {
		c.t.Fatalf("the stream ended before its live marker: %v", summary(frames))
	}
	return frames
}

// transcript reads frames until n transcript events were read, and
// returns them.
func (c *sseClient) transcript(n int) []contract.Event {
	c.t.Helper()
	var got []contract.Event
	if n == 0 {
		return got
	}
	_, ok := c.until(func(f frame) bool {
		if f.Event == contract.SSETranscript {
			got = append(got, f.event(c.t))
		}
		return len(got) == n
	})
	if !ok {
		c.t.Fatalf("the stream ended after %d of %d events", len(got), n)
	}
	return got
}

// summary lists the frames' event names, for a failure message.
func summary(frames []frame) []string {
	names := make([]string, len(frames))
	for i, f := range frames {
		names[i] = f.Event + " " + f.ID
	}
	return names
}

// ofEvent filters frames by SSE event name.
func ofEvent(frames []frame, event string) []frame {
	var found []frame
	for _, f := range frames {
		if f.Event == event {
			found = append(found, f)
		}
	}
	return found
}

// transcriptOf decodes the transcript frames.
func transcriptOf(t *testing.T, frames []frame) []contract.Event {
	t.Helper()
	var events []contract.Event
	for _, f := range ofEvent(frames, contract.SSETranscript) {
		events = append(events, f.event(t))
	}
	return events
}

// payloadJSON re-encodes v, for comparisons of whole documents.
func payloadJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// validateUnless checks data against def, unless skip.
func validateUnless(skip bool, def, data string) error {
	if skip {
		return nil
	}
	return contract.Validate(def, []byte(data))
}
