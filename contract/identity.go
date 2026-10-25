package contract

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Identity patterns, kept identical to the schema's DNSLabel and UID.
var (
	dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	uidPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Separators of the attempt and event identifiers. Neither can occur in a
// UID or a DNS label, so both identifiers split without ambiguity.
const (
	attemptSeparator = "."
	eventSeparator   = ":"
	gapPart          = "gap"
	maxRestart       = 9999999999
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
	restart, err := parseCanonicalUint(parts[2])
	if err != nil {
		return AttemptKey{}, fmt.Errorf("%w: attempt %q restart: %v", ErrInvalidKey, id, err)
	}
	key := AttemptKey{PodUID: parts[0], Container: parts[1], Restart: restart}
	if err := key.Validate(); err != nil {
		return AttemptKey{}, err
	}
	return key, nil
}

// EventID is the identifier of the event at part of line in an attempt.
func EventID(attemptID string, line int64, part int) string {
	return attemptID + eventSeparator + strconv.FormatInt(line, 10) + eventSeparator + strconv.Itoa(part)
}

// GapEventID is the identifier of a gap found before line in an attempt.
func GapEventID(attemptID string, line int64) string {
	return attemptID + eventSeparator + strconv.FormatInt(line, 10) + eventSeparator + gapPart
}

// parseCanonicalUint accepts only the decimal form FormatInt produces, so
// an identifier has exactly one spelling.
func parseCanonicalUint(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("%q is not a canonical non-negative integer", s)
	}
	return n, nil
}
