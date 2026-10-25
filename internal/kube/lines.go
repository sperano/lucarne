package kube

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"time"

	"github.com/sperano/lucarne/contract"
)

const (
	// readBufferBytes is the buffer a log stream is read through.
	readBufferBytes = 64 << 10
	// maxPrefixBytes bounds the timestamp prefix the log API writes
	// before each line when asked to: RFC 3339 with nanoseconds and an
	// offset, then a space.
	maxPrefixBytes = len(time.RFC3339Nano) + 1
	// originBytes is how much of a first line's SHA-256 its origin keeps,
	// as contract.LogOrigin does.
	originBytes = 8
)

// rawLine is one line as the log API returned it.
type rawLine struct {
	// time is the timestamp prefix, nil when the line had none.
	time *time.Time
	// text is the line without prefix and ending, at most
	// contract.MaxLineBytes of it.
	text []byte
	// bytes is the length of the whole text, before the cut.
	bytes int64
	// size is what the line took in the stream, prefix and ending included.
	size int64
	// sum is the SHA-256 of the line, prefix included, ending excluded:
	// what its origin is computed from, and how an overlap finds it again.
	sum [sha256.Size]byte
	// ended is false for a last line the stream ended without a newline.
	ended bool
}

// origin is the log origin of a read whose first line is l.
func (l rawLine) origin() string { return hex.EncodeToString(l.sum[:originBytes]) }

// lineReader splits a log stream into lines, holding at most
// contract.MaxLineBytes of each.
type lineReader struct {
	r    *bufio.Reader
	head []byte
	hash hash.Hash
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{r: bufio.NewReaderSize(r, readBufferBytes), hash: sha256.New()}
}

// next returns the next line. At the end of the stream it returns the last
// line without a newline, if any, then io.EOF; a broken stream's error
// comes after the lines read before it.
func (lr *lineReader) next() (rawLine, error) {
	lr.head = lr.head[:0]
	lr.hash.Reset()
	var size int64
	for {
		chunk, err := lr.r.ReadSlice('\n')
		size += int64(len(chunk))
		ended := err == nil
		if ended {
			chunk = chunk[:len(chunk)-1]
		}
		lr.hash.Write(chunk)
		if room := maxPrefixBytes + contract.MaxLineBytes - len(lr.head); room > 0 {
			lr.head = append(lr.head, chunk[:min(room, len(chunk))]...)
		}
		switch {
		case ended:
			return lr.line(size, true), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case size > 0 && errors.Is(err, io.EOF):
			return lr.line(size, false), nil
		default:
			return rawLine{}, err
		}
	}
}

// line builds the line read into head, size bytes long in the stream.
func (lr *lineReader) line(size int64, ended bool) rawLine {
	l := rawLine{size: size, ended: ended}
	lr.hash.Sum(l.sum[:0])
	whole := size
	if ended {
		whole--
	}
	text := lr.head
	if t, rest, ok := cutTimestamp(text); ok {
		l.time, text = &t, rest
		whole -= int64(len(lr.head) - len(rest))
	}
	l.bytes = whole
	l.text = bytes.Clone(text[:min(len(text), contract.MaxLineBytes)])
	return l
}

// cutTimestamp splits the log API's timestamp prefix off a line.
func cutTimestamp(line []byte) (time.Time, []byte, bool) {
	space := bytes.IndexByte(line[:min(len(line), maxPrefixBytes)], ' ')
	if space <= 0 {
		return time.Time{}, line, false
	}
	t, err := time.Parse(time.RFC3339Nano, string(line[:space]))
	if err != nil {
		return time.Time{}, line, false
	}
	return t.UTC(), line[space+1:], true
}
