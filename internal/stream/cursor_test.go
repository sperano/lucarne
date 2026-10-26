package stream

import "testing"

func TestParseCursor(t *testing.T) {
	t.Parallel()
	valid := map[string]Cursor{
		"k3v9q2:0":  {Epoch: "k3v9q2", Seq: 0},
		"0a:42":     {Epoch: "0a", Seq: 42},
		"z:1234567": {Epoch: "z", Seq: 1234567},
	}
	for text, want := range valid {
		got, ok := ParseCursor(text)
		if !ok || got != want || got.String() != text {
			t.Errorf("ParseCursor(%q) = %+v, %v; want %+v", text, got, ok, want)
		}
	}
	for _, text := range []string{
		"", ":1", "e", "e:", "e:01", "e:-1", "e:+1", "e:1:2", "E:1", "e-1:1", "é:1",
		"0123456789abcdefghijklmnopqrstuvw:1", "e:99999999999999999999",
	} {
		if c, ok := ParseCursor(text); ok {
			t.Errorf("ParseCursor(%q) = %+v, want rejected", text, c)
		}
	}
}

func TestNewEpochIsAValidEpoch(t *testing.T) {
	t.Parallel()
	a, b := newEpoch(), newEpoch()
	if !validEpoch(a) || a == b {
		t.Errorf("epochs %q and %q, want two distinct valid ones", a, b)
	}
}
