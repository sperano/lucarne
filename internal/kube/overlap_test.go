package kube

import (
	"strings"
	"testing"
	"time"
)

func parsed(t *testing.T, stream string) []rawLine {
	t.Helper()
	got, _ := readAll(t, strings.NewReader(stream))
	return got
}

func tailOf(t *testing.T, stream string) *tail {
	t.Helper()
	var tl tail
	for _, l := range parsed(t, stream) {
		tl.add(l)
	}
	return &tl
}

// TestTailKeepsLastSecond proves the tail holds the lines of the last line's
// second only, which a read since that second returns.
func TestTailKeepsLastSecond(t *testing.T) {
	second := baseTime.Add(time.Second)
	tl := tailOf(t, logLine(baseTime, "a")+logLine(second, "b")+logLine(second.Add(time.Millisecond), "c"))
	if len(tl.sums) != 2 || !tl.since.Equal(second) || !tl.last.Equal(second.Add(time.Millisecond)) {
		t.Errorf("tail = %d lines since %v, last %v", len(tl.sums), tl.since, tl.last)
	}
}

func TestTailBounded(t *testing.T) {
	var tl tail
	for _, l := range parsed(t, lines(baseTime, 0, maxOverlapLines+10)) {
		l.time = &baseTime
		tl.add(l)
	}
	if len(tl.sums) != maxOverlapLines {
		t.Errorf("tail holds %d lines", len(tl.sums))
	}
}

// steps classifies each line of a resumed stream, up to the tail's end.
func steps(t *testing.T, tl *tail, stream string) []overlapStep {
	t.Helper()
	r := newResume(tl)
	var got []overlapStep
	for _, l := range parsed(t, stream) {
		step := r.step(l)
		got = append(got, step)
		if step != overlapSkip {
			break
		}
	}
	return got
}

func TestResume(t *testing.T) {
	at := baseTime.Add(500 * time.Millisecond)
	// Two identical lines, the tail; the line before is in the second before.
	tl := tailOf(t, logLine(baseTime.Add(-time.Millisecond), "before")+logLine(at, "same")+logLine(at, "same"))
	cases := map[string]struct {
		stream string
		want   []overlapStep
	}{
		"found":             {logLine(at, "same") + logLine(at, "same") + logLine(at, "new"), []overlapStep{overlapSkip, overlapFound}},
		"older line first":  {logLine(baseTime, "older") + logLine(at, "same") + logLine(at, "same"), []overlapStep{overlapSkip, overlapSkip, overlapFound}},
		"only one repeat":   {logLine(at, "same"), []overlapStep{overlapSkip}},
		"lines lost":        {logLine(at.Add(time.Second), "after"), []overlapStep{overlapLost}},
		"half then newer":   {logLine(at, "same") + logLine(at.Add(time.Second), "new"), []overlapStep{overlapSkip, overlapLost}},
		"other at the time": {logLine(at, "other") + logLine(at, "same") + logLine(at, "same"), []overlapStep{overlapSkip, overlapSkip, overlapFound}},
		"untimed":           {"no prefix\n", []overlapStep{overlapLost}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := steps(t, tl, tc.stream)
			if len(got) != len(tc.want) {
				t.Fatalf("steps = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("steps = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
