package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Identity patterns, compiled from the schema's own DNSLabel and UID so that
// a key the server accepts is one the schema and the UI accept.
var (
	dnsLabelPattern = schemaPattern("DNSLabel")
	uidPattern      = schemaPattern("UID")
)

// Separators of the attempt and event identifiers. Neither can occur in a
// UID or a DNS label, so both identifiers split without ambiguity.
const (
	attemptSeparator = "."
	eventSeparator   = ":"
	// gapPartPrefix marks a gap event's part, so that a gap and an event
	// the same line produced never share an ID.
	gapPartPrefix = "gap."
	maxRestart    = 9999999999
	// originBytes is how much of the first line's SHA-256 a log origin keeps:
	// 64 bits, written as 16 hex digits.
	originBytes = 8
)

// ErrInvalidKey is wrapped by every identity parse or validation error.
var ErrInvalidKey = errors.New("invalid key")

// RunKey identifies one run: a Kelos Task by its UID, never by its name,
// which a retry or a later Task reuses.
type RunKey struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	TaskUID   string `json:"taskUID"`
}

// Validate reports whether every part of the key has its canonical form.
func (k RunKey) Validate() error {
	if !dnsLabelPattern.MatchString(k.Cluster) {
		return fmt.Errorf("%w: cluster %q is not a DNS label", ErrInvalidKey, k.Cluster)
	}
	if !dnsLabelPattern.MatchString(k.Namespace) {
		return fmt.Errorf("%w: namespace %q is not a DNS label", ErrInvalidKey, k.Namespace)
	}
	if !uidPattern.MatchString(k.TaskUID) {
		return fmt.Errorf("%w: task UID %q is not a lowercase UID", ErrInvalidKey, k.TaskUID)
	}
	return nil
}

// path is the key as it appears in deep links and API paths.
func (k RunKey) path() string {
	return k.Cluster + "/" + k.Namespace + "/" + k.TaskUID
}

// LinkPath is the deep link that opens this run in the UI.
func (k RunKey) LinkPath() string { return "/runs/" + k.path() }

// SnapshotPath is the API path of this run's snapshot.
func (k RunKey) SnapshotPath() string { return APIPrefix + "/runs/" + k.path() }

// EventsPath is the API path of this run's SSE stream.
func (k RunKey) EventsPath() string { return k.SnapshotPath() + "/events" }

// AttemptKey identifies one container instance of a run: a pod, one of its
// containers, and that container's restart count when it ran.
type AttemptKey struct {
	PodUID    string `json:"podUID"`
	Container string `json:"container"`
	Restart   int64  `json:"restart"`
}

// Validate reports whether every part of the key has its canonical form.
func (a AttemptKey) Validate() error {
	if !uidPattern.MatchString(a.PodUID) {
		return fmt.Errorf("%w: pod UID %q is not a lowercase UID", ErrInvalidKey, a.PodUID)
	}
	if !dnsLabelPattern.MatchString(a.Container) {
		return fmt.Errorf("%w: container %q is not a DNS label", ErrInvalidKey, a.Container)
	}
	if a.Restart < 0 || a.Restart > maxRestart {
		return fmt.Errorf("%w: restart %d out of range", ErrInvalidKey, a.Restart)
	}
	return nil
}

// ID is the attempt's identifier: <podUID>.<container>.<restart>.
func (a AttemptKey) ID() string {
	return a.PodUID + attemptSeparator + a.Container + attemptSeparator + strconv.FormatInt(a.Restart, 10)
}

// ParseAttemptID is the inverse of AttemptKey.ID.
func ParseAttemptID(id string) (AttemptKey, error) {
	parts := strings.Split(id, attemptSeparator)
	if len(parts) != 3 {
		return AttemptKey{}, fmt.Errorf("%w: attempt %q has %d parts, want 3", ErrInvalidKey, id, len(parts))
	}
	restart, err := parseCanonicalNonNegative(parts[2])
	if err != nil {
		return AttemptKey{}, fmt.Errorf("%w: attempt %q restart: %v", ErrInvalidKey, id, err)
	}
	key := AttemptKey{PodUID: parts[0], Container: parts[1], Restart: restart}
	if err := key.Validate(); err != nil {
		return AttemptKey{}, err
	}
	return key, nil
}

// LogOrigin names the start of one read of an attempt's log: the first line
// read, as the log API returned it (timestamp prefix included, line ending
// excluded). Line ordinals count from that line, so the origin changes
// whenever they do, as when rotation removed the log's start before a re-read.
func LogOrigin(firstLine []byte) string {
	sum := sha256.Sum256(firstLine)
	return hex.EncodeToString(sum[:originBytes])
}

// EventID is the identifier of the event at part of line in an attempt's log
// read from origin.
func EventID(attemptID, origin string, line int64, part int) string {
	return eventPrefix(attemptID, origin, line) + strconv.Itoa(part)
}

// GapEventID is the identifier of the part-th gap found before line in an
// attempt's log read from origin. A gap after the last line read is before
// the next line, lines read + 1.
func GapEventID(attemptID, origin string, line int64, part int) string {
	return eventPrefix(attemptID, origin, line) + gapPartPrefix + strconv.Itoa(part)
}

// eventPrefix is the part of an event ID before the part number.
func eventPrefix(attemptID, origin string, line int64) string {
	return attemptID + eventSeparator + origin + eventSeparator + strconv.FormatInt(line, 10) + eventSeparator
}

// parseCanonicalNonNegative accepts only the decimal form FormatInt produces, so
// an identifier has exactly one spelling.
func parseCanonicalNonNegative(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("%q is not a canonical non-negative integer", s)
	}
	return n, nil
}

// schemaPattern compiles the pattern of one string definition of the
// embedded schema. It panics, as regexp.MustCompile does, when the
// definition has no pattern Go can compile.
func schemaPattern(def string) *regexp.Regexp {
	var doc struct {
		Defs map[string]struct {
			Pattern string `json:"pattern"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON, &doc); err != nil {
		panic(fmt.Sprintf("contract: decode schema: %v", err))
	}
	pattern := doc.Defs[def].Pattern
	if pattern == "" {
		panic(fmt.Sprintf("contract: $defs/%s has no pattern", def))
	}
	return regexp.MustCompile(pattern)
}
