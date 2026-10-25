package codex

import (
	"crypto/sha256"
	"encoding/binary"
)

// maxSeenItems bounds the items whose rendering the parser remembers. An
// item evicted, or one from before the log read began, renders its next
// line in full again: a repeat is shown rather than a change lost.
const maxSeenItems = 4096

// digest stands for rendered content, so that repeats are found without
// holding the content.
type digest [sha256.Size]byte

// shown is what the parser last rendered of one item. The zero value is
// an item nothing was rendered of.
type shown struct {
	// content is the digest of the message or error item's content, or
	// of the tool call's name and input.
	content digest
	// outcome is the digest of the tool result rendered.
	outcome digest
	// hasContent and hasOutcome say whether content and outcome are set.
	hasContent, hasOutcome bool
}

// seenItems maps item IDs to what was rendered of them, oldest first.
type seenItems struct {
	items map[string]*shown
	order []string
}

func newSeenItems() *seenItems { return &seenItems{items: make(map[string]*shown)} }

// get returns the item's record, adding an empty one, and evicting the
// oldest when maxSeenItems are held, when the ID is new.
func (s *seenItems) get(id string) *shown {
	if item, ok := s.items[id]; ok {
		return item
	}
	if len(s.order) == maxSeenItems {
		delete(s.items, s.order[0])
		// The append below reallocates when the slice runs out of
		// capacity, copying only the IDs still held.
		s.order = s.order[1:]
	}
	item := &shown{}
	s.items[id] = item
	s.order = append(s.order, id)
	return item
}

// sameContent records d as the item's content and reports whether it was
// already.
func (s *shown) sameContent(d digest) bool {
	same := s.hasContent && s.content == d
	s.content, s.hasContent = d, true
	return same
}

// sameOutcome records d as the item's outcome and reports whether it was
// already.
func (s *shown) sameOutcome(d digest) bool {
	same := s.hasOutcome && s.outcome == d
	s.outcome, s.hasOutcome = d, true
	return same
}

// digestOf is the digest of parts, each length-prefixed so that no two
// sequences collide by concatenation.
func digestOf(parts ...string) digest {
	h := sha256.New()
	for _, p := range parts {
		var size [8]byte
		binary.LittleEndian.PutUint64(size[:], uint64(len(p)))
		h.Write(size[:])
		h.Write([]byte(p))
	}
	var d digest
	h.Sum(d[:0])
	return d
}
