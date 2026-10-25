package codex

import (
	"strconv"
	"testing"
)

func TestSeenItemsAreBounded(t *testing.T) {
	t.Parallel()
	s := newSeenItems()
	s.get("first").sameContent(digestOf("a"))
	for i := range maxSeenItems * 3 {
		s.get(strconv.Itoa(i))
	}
	if len(s.items) != maxSeenItems || len(s.order) != maxSeenItems {
		t.Errorf("%d items and %d IDs held, want %d", len(s.items), len(s.order), maxSeenItems)
	}
	if cap(s.order) > 2*maxSeenItems {
		t.Errorf("order's capacity grew to %d", cap(s.order))
	}
	if s.get("first").sameContent(digestOf("a")) {
		t.Error("the oldest item was not evicted")
	}
}

func TestShownRecordsTheLatest(t *testing.T) {
	t.Parallel()
	var s shown
	if s.sameContent(digestOf("a")) || !s.sameContent(digestOf("a")) || s.sameContent(digestOf("b")) || s.sameContent(digestOf("a")) {
		t.Error("sameContent does not compare with the latest digest")
	}
	if s.sameOutcome(digestOf("a")) || !s.sameOutcome(digestOf("a")) {
		t.Error("sameOutcome does not record its digest")
	}
}

func TestDigestOfSeparatesParts(t *testing.T) {
	t.Parallel()
	if digestOf("ab", "c") == digestOf("a", "bc") || digestOf("a") == digestOf("a", "") {
		t.Error("two part sequences with the same concatenation share a digest")
	}
}
