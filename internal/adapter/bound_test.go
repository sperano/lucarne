package adapter

import (
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
)

func TestCut(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		limit int
		want  string
		cut   bool
	}{
		{"abc", 3, "abc", false},
		{"abcd", 3, "abc", true},
		{"aé", 2, "a", true},
		{"日本", 5, "日", true},
		{"a\xffb", 5, "a�b", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		got, cut := Cut(c.in, c.limit)
		if got != c.want || cut != c.cut || !utf8.ValidString(got) {
			t.Errorf("Cut(%q, %d) = %q, %v; want %q, %v", c.in, c.limit, got, cut, c.want, c.cut)
		}
	}
}

func TestTruncationNamesEachFieldOnceWithinTheLimit(t *testing.T) {
	t.Parallel()
	var cut Truncation
	cut.Text("text", "abc", 1)
	cut.Text("text", "abc", 1)
	cut.Text("other", "a", 1)
	if got := cut.Fields(); len(got) != 1 || got[0] != "text" {
		t.Errorf("fields = %v, want [text]", got)
	}
	for i := range contract.MaxTruncatedFields * 2 {
		cut.Mark("field" + strconv.Itoa(i))
	}
	if n := len(cut.Fields()); n != contract.MaxTruncatedFields {
		t.Errorf("%d fields, want at most %d", n, contract.MaxTruncatedFields)
	}
	if got := cut.Optional("absent", nil, 1); got != nil {
		t.Errorf("Optional(nil) = %q, want nil", *got)
	}
}
