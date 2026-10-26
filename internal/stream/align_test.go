package stream

import (
	"testing"

	"github.com/sperano/lucarne/internal/stream/streamtest"
)

func TestAlignmentFirstReadKeepsTheSourceNumbers(t *testing.T) {
	t.Parallel()
	al := alignment{}
	al.startRead()
	for _, l := range streamtest.Lines(1, 3) {
		if n, lost := al.place(l); n != l.Number || lost {
			t.Fatalf("line %d placed at %d, lost %v", l.Number, n, lost)
		}
	}
	if al.gapBefore(4) != 4 || al.unresolved() {
		t.Error("a first read is not aligned")
	}
}

func TestAlignmentRereadSkipsUpToTheLastLine(t *testing.T) {
	t.Parallel()
	al := alignment{}
	al.startRead()
	for _, l := range streamtest.Lines(1, 3) {
		al.place(l)
	}
	al.startRead()
	if al.gapBefore(2) != 0 {
		t.Error("a gap among the lines delivered was kept")
	}
	for _, l := range streamtest.Lines(1, 3) {
		if n, _ := al.place(l); n != 0 {
			t.Fatalf("line %d delivered again as %d", l.Number, n)
		}
	}
	if n, lost := al.place(streamtest.Line(4, `{"n":4}`)); n != 4 || lost {
		t.Errorf("new line placed at %d, lost %v; want 4", n, lost)
	}
}

func TestAlignmentReadEndingBeforeTheLastLineIsResolved(t *testing.T) {
	t.Parallel()
	al := alignment{}
	al.startRead()
	for _, l := range streamtest.Lines(1, 3) {
		al.place(l)
	}
	al.startRead()
	al.place(streamtest.Line(1, `{"n":1}`))
	if al.unresolved() {
		t.Error("a read that did not reach the last line delivered is unresolved")
	}
}
