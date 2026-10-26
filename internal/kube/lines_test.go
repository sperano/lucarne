package kube

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
)

func readAll(t *testing.T, r io.Reader) ([]rawLine, error) {
	t.Helper()
	lr := newLineReader(r)
	var got []rawLine
	for {
		l, err := lr.next()
		if err != nil {
			return got, err
		}
		got = append(got, l)
	}
}

func TestLineReaderSplits(t *testing.T) {
	t.Parallel()
	at := baseTime.Add(123456789 * time.Nanosecond)
	stream := logLine(at, `{"type":"system"}`) + logLine(at, "") + logLine(at, "carriage\r") + "no prefix\n" + logLine(at, "last")
	stream = strings.TrimSuffix(stream, "\n")
	got, err := readAll(t, strings.NewReader(stream))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v", err)
	}
	want := []struct {
		text  string
		timed bool
		ended bool
	}{
		{`{"type":"system"}`, true, true}, {"", true, true}, {"carriage\r", true, true}, {"no prefix", false, true}, {"last", true, false},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d", len(got), len(want))
	}
	for i, w := range want {
		l := got[i]
		if string(l.text) != w.text || (l.time != nil) != w.timed || l.ended != w.ended || l.bytes != int64(len(w.text)) {
			t.Errorf("line %d = %q time %v ended %v bytes %d, want %+v", i, l.text, l.time, l.ended, l.bytes, w)
		}
		if w.timed && !l.time.Equal(at) {
			t.Errorf("line %d time = %v", i, l.time)
		}
	}
	first := strings.TrimSuffix(logLine(at, `{"type":"system"}`), "\n")
	if got[0].origin() != contract.LogOrigin([]byte(first)) {
		t.Errorf("origin = %s, want contract.LogOrigin of the line as returned", got[0].origin())
	}
	if got[0].size != int64(len(first))+1 {
		t.Errorf("size = %d", got[0].size)
	}
}

// TestLineReaderOversized proves a line longer than the limit is held only
// to the limit, while its length and origin cover all of it.
func TestLineReaderOversized(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("x", contract.MaxLineBytes+readBufferBytes+1)
	line := logLine(baseTime, text)
	got, err := readAll(t, strings.NewReader(line+logLine(baseTime, "next")))
	if !errors.Is(err, io.EOF) || len(got) != 2 {
		t.Fatalf("lines %d, err %v", len(got), err)
	}
	l := got[0]
	if len(l.text) != contract.MaxLineBytes || l.bytes != int64(len(text)) || l.time == nil {
		t.Errorf("held %d bytes of %d, time %v", len(l.text), l.bytes, l.time)
	}
	if l.origin() != contract.LogOrigin([]byte(strings.TrimSuffix(line, "\n"))) {
		t.Error("origin does not cover the whole line")
	}
	if string(got[1].text) != "next" {
		t.Errorf("next line = %q", got[1].text)
	}
}

// TestLineReaderBrokenStream proves a stream that breaks mid-line returns
// its error, not the cut line.
func TestLineReaderBrokenStream(t *testing.T) {
	t.Parallel()
	broken := errors.New("connection reset")
	stream := io.MultiReader(strings.NewReader(logLine(baseTime, "whole")+"2026-10-25T10:00:00Z cut"), errReader{broken})
	got, err := readAll(t, stream)
	if !errors.Is(err, broken) || len(got) != 1 {
		t.Errorf("lines %d, err %v", len(got), err)
	}
}

func TestCutTimestamp(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"2026-10-25T10:00:00Z text":           true,
		"2026-10-25T10:00:00.5+02:00 text":    true,
		"2026-10-25T10:00:00Z":                false,
		"not-a-time text":                     false,
		" leading space":                      false,
		"{\"time\":\"2026-10-25T10:00:00Z\"}": false,
	}
	for line, ok := range cases {
		if _, _, got := cutTimestamp([]byte(line)); got != ok {
			t.Errorf("cutTimestamp(%q) = %v, want %v", line, got, ok)
		}
	}
}
