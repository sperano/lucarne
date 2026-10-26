package stream

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/sperano/lucarne/internal/adapter"
)

// maxRealignLines is how many lines from the position it stopped at a read
// of a log again may search for the last line delivered before it gives
// up: a first read that lost lines has them again in the re-read, so the
// line comes later than its number.
const maxRealignLines = 1024

// fingerprint identifies a line by its receipt time, length and text: two
// identical lines written at different times differ.
type fingerprint [sha256.Size]byte

func fingerprintOf(l adapter.Line) fingerprint {
	h := sha256.New()
	if l.LogTime != nil {
		h.Write([]byte(l.LogTime.UTC().Format(time.RFC3339Nano)))
	}
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(l.Bytes)))
	h.Write(l.Text)
	var f fingerprint
	h.Sum(f[:0])
	return f
}

// alignment numbers the lines of the reads of one log from one origin. The
// first read numbers them as the source does. A read again from the start
// is lined up on the last line delivered: the lines up to it were
// delivered already, the ones after it are new and numbered on.
type alignment struct {
	// delivered is the number of the last line delivered, last its
	// fingerprint.
	delivered int64
	last      fingerprint

	// aligned is a read past the lines already delivered; a line of it
	// then gets its number plus offset.
	aligned bool
	offset  int64
	// searched counts the lines compared with last in this read.
	searched int
}

// startRead prepares for a read of the log from its start.
func (al *alignment) startRead() {
	al.aligned, al.offset, al.searched = al.delivered == 0, 0, 0
}

// place returns the number line gets, 0 for one delivered already. lost
// reports a read that could not be lined up within maxRealignLines: the
// line follows the last delivered one, and lines may be missing or
// repeated between them.
func (al *alignment) place(line adapter.Line) (number int64, lost bool) {
	f := fingerprintOf(line)
	if !al.aligned {
		if line.Number < al.delivered {
			return 0, false
		}
		if f == al.last {
			al.aligned, al.offset = true, al.delivered-line.Number
			return 0, false
		}
		if al.searched++; al.searched < maxRealignLines {
			return 0, false
		}
		al.aligned, al.offset, lost = true, al.delivered+1-line.Number, true
	}
	al.delivered, al.last = line.Number+al.offset, f
	return al.delivered, lost
}

// gapBefore returns the number of the line a gap the read found before
// its line next is before, 0 for one among the lines delivered already.
func (al *alignment) gapBefore(next int64) int64 {
	if !al.aligned {
		return 0
	}
	return next + al.offset
}

// unresolved is a read that passed the last line delivered without
// finding it again.
func (al *alignment) unresolved() bool {
	return !al.aligned && al.searched > 0
}
