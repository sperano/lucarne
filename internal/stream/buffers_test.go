package stream

import (
	"slices"
	"testing"
)

func TestDequeWrapsAround(t *testing.T) {
	t.Parallel()
	var d deque[int]
	var want []int
	for i := range 3 * minDequeSize {
		d.push(i)
		want = append(want, i)
		if i%3 == 0 {
			if got := d.pop(); got != want[0] {
				t.Fatalf("pop %d, want %d", got, want[0])
			}
			want = want[1:]
		}
	}
	got := make([]int, d.len())
	for i := range got {
		got[i] = d.at(i)
	}
	if !slices.Equal(got, want) {
		t.Errorf("deque holds %v, want %v", got, want)
	}
}

func updatesTo(last int64) *replay {
	b := &replay{bounds: Bounds{ReplayUpdates: 3, RunBytes: 1 << 20}}
	for seq := int64(1); seq <= last; seq++ {
		b.add(&Update{seq: seq, Data: []byte("{}")})
	}
	return b
}

func seqs(updates []*Update) []int64 {
	out := make([]int64, len(updates))
	for i, u := range updates {
		out[i] = u.seq
	}
	return out
}

func TestReplayAfter(t *testing.T) {
	t.Parallel()
	b := updatesTo(5)
	tests := []struct {
		after int64
		want  []int64
		ok    bool
	}{
		{after: 5, want: []int64{}, ok: true},
		{after: 4, want: []int64{5}, ok: true},
		{after: 2, want: []int64{3, 4, 5}, ok: true},
		{after: 1, ok: false},
		{after: 0, ok: false},
	}
	for _, tt := range tests {
		got, ok := b.after(tt.after, 5)
		if ok != tt.ok || (ok && !slices.Equal(seqs(got), tt.want)) {
			t.Errorf("after(%d) = %v, %v; want %v, %v", tt.after, seqs(got), ok, tt.want, tt.ok)
		}
	}
	if _, ok := (&replay{}).after(0, 1); ok {
		t.Error("an empty replay served a missing update")
	}
}

func TestReplayBoundedByBytesKeepsTheNewest(t *testing.T) {
	t.Parallel()
	b := &replay{bounds: Bounds{ReplayUpdates: 10, RunBytes: 3}}
	b.add(&Update{seq: 1, Data: []byte("ab")})
	b.add(&Update{seq: 2, Data: []byte("abcd")})
	if b.updates.len() != 1 || b.updates.at(0).seq != 2 {
		t.Errorf("replay holds %d updates, want the newest alone", b.updates.len())
	}
}
