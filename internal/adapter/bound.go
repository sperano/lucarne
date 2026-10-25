package adapter

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sperano/lucarne/contract"
)

// Cut returns s with invalid UTF-8 replaced, cut to at most limit bytes at
// a UTF-8 boundary, and whether it had to cut.
func Cut(s string, limit int) (string, bool) {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	if len(s) <= limit {
		return s, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end], true
}

// Truncation collects the payload fields an event cut to their limit, for
// its Truncated list.
type Truncation struct {
	fields []string
}

// Text returns s cut to limit bytes and records field when it was cut.
func (t *Truncation) Text(field, s string, limit int) string {
	cut, ok := Cut(s, limit)
	if ok {
		t.Mark(field)
	}
	return cut
}

// Optional is Text for a field the source may not have carried: nil stays
// nil.
func (t *Truncation) Optional(field string, s *string, limit int) *string {
	if s == nil {
		return nil
	}
	cut := t.Text(field, *s, limit)
	return &cut
}

// Mark records that field was cut. The list holds each name once and at
// most contract.MaxTruncatedFields names.
func (t *Truncation) Mark(field string) {
	if !slices.Contains(t.fields, field) && len(t.fields) < contract.MaxTruncatedFields {
		t.fields = append(t.fields, field)
	}
}

// Fields is the Truncated list, nil when nothing was cut.
func (t *Truncation) Fields() []string { return t.fields }
