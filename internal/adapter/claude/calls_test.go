package claude

import (
	"strconv"
	"testing"
)

func TestOpenCallsAreBounded(t *testing.T) {
	t.Parallel()
	c := newOpenCalls()
	c.open("stuck", "Bash")
	for i := range orderCompactFactor * maxOpenCalls * 2 {
		id := strconv.Itoa(i)
		c.open(id, "Read")
		if c.close(id) != "Read" {
			t.Fatalf("call %s lost its name", id)
		}
	}
	if len(c.order) > orderCompactFactor*maxOpenCalls {
		t.Errorf("order holds %d IDs, want at most %d", len(c.order), orderCompactFactor*maxOpenCalls)
	}
	for i := range maxOpenCalls {
		c.open("new"+strconv.Itoa(i), "Edit")
	}
	if len(c.names) != maxOpenCalls {
		t.Errorf("%d calls open, want %d", len(c.names), maxOpenCalls)
	}
	if c.close("stuck") != "" {
		t.Error("the oldest call was not evicted")
	}
	if c.close("new0") != "Edit" {
		t.Error("a call newer than the evicted one was lost")
	}
}

func TestOpenCallReopened(t *testing.T) {
	t.Parallel()
	c := newOpenCalls()
	c.open("a", "Read")
	c.open("a", "Edit")
	if got := c.close("a"); got != "Edit" {
		t.Errorf("close = %q, want the latest name", got)
	}
	if got := c.close("a"); got != "" {
		t.Errorf("a closed call closed again gave %q", got)
	}
}
