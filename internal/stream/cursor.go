package stream

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
)

const (
	// epochBytes is how many random bytes name an epoch, written as hex:
	// the schema's Epoch allows [0-9a-z]{1,32}.
	epochBytes = 8
	// maxEpochLength is the schema's longest Epoch.
	maxEpochLength = 32
	// cursorSeparator splits a cursor's epoch from its seq.
	cursorSeparator = ":"
)

// Cursor is a stream position: an epoch and the last seq a client holds.
type Cursor struct {
	Epoch string
	Seq   int64
}

// String is the cursor as an SSE id and an after parameter: epoch:seq.
func (c Cursor) String() string {
	return c.Epoch + cursorSeparator + strconv.FormatInt(c.Seq, 10)
}

// ParseCursor reads a cursor in its canonical form only, so that each
// position has one spelling.
func ParseCursor(s string) (Cursor, bool) {
	epoch, seq, ok := strings.Cut(s, cursorSeparator)
	if !ok || !validEpoch(epoch) {
		return Cursor{}, false
	}
	n, err := strconv.ParseInt(seq, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != seq {
		return Cursor{}, false
	}
	return Cursor{Epoch: epoch, Seq: n}, true
}

// validEpoch checks the schema's Epoch pattern, ^[0-9a-z]{1,32}$.
func validEpoch(epoch string) bool {
	if epoch == "" || len(epoch) > maxEpochLength {
		return false
	}
	for _, c := range epoch {
		if (c < '0' || c > '9') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

// newEpoch names a new history. Epochs are random, so a restarted server
// never reuses one.
func newEpoch() string {
	b := make([]byte, epochBytes)
	// crypto/rand.Read never returns an error.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
