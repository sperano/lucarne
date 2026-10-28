package stream

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

func refOf(t *testing.T, id string) contract.EventRef {
	t.Helper()
	ref, err := contract.ParseEventID(id)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func sourceOf(t *testing.T, h *Hub, id string) (contract.SourceLine, error) {
	t.Helper()
	data, err := h.Source(context.Background(), streamtest.Key, refOf(t, id))
	if err != nil {
		return contract.SourceLine{}, err
	}
	if err := contract.Validate(contract.DefSourceLine, data); err != nil {
		t.Fatal(err)
	}
	return decode[contract.SourceLine](t, data), nil
}

func TestSourceServesTheLineOfEachEvent(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("source")
	src := source(contract.PhaseSucceeded, terminated())
	// Two identical lines are two lines; a blank one produces no event.
	lines := []adapter.Line{
		streamtest.Line(1, `{"same":true}`), streamtest.Line(2, ""), streamtest.Line(3, `{"same":true}`),
	}
	src.AddReads(attemptID, streamtest.Deliver(origin, lines, kube.EndComplete, ""))
	h := newHub(t, src)
	snap := settled(t, h)
	if len(snap.Events) != 2 {
		t.Fatalf("events %v, want 2", ids(snap.Events))
	}
	for _, ev := range snap.Events {
		got, err := sourceOf(t, h, ev.ID)
		if err != nil {
			t.Fatalf("source of %s: %v", ev.ID, err)
		}
		want := contract.SourceLine{
			Schema: contract.SchemaVersion, Attempt: attemptID, Origin: origin,
			Line: ev.Source.Line, Text: `{"same":true}`, Bytes: int64(len(`{"same":true}`)),
		}
		if got != want {
			t.Errorf("source of %s = %+v, want %+v", ev.ID, got, want)
		}
	}
	if _, err := sourceOf(t, h, contract.EventID(attemptID, origin, 2, 0)); !errors.Is(err, ErrSourceNotFound) {
		t.Errorf("blank line: err %v, want ErrSourceNotFound", err)
	}
}

func TestSourceAnswersNotFoundForWhatNoLineHolds(t *testing.T) {
	t.Parallel()
	origin := streamtest.Origin("source")
	src := source(contract.PhaseSucceeded, terminated())
	src.AddReads(attemptID, complete(origin, 1, 2))
	h := newHub(t, src)
	settled(t, h)
	other := streamtest.Attempt(streamtest.RetryPodUID, 0, contract.ContainerTerminated).ID
	for name, id := range map[string]string{
		"gap":           contract.GapEventID(attemptID, origin, 1, 0),
		"no such line":  contract.EventID(attemptID, origin, 3, 0),
		"other origin":  contract.EventID(attemptID, streamtest.Origin("rotated"), 1, 0),
		"other attempt": contract.EventID(other, origin, 1, 0),
	} {
		if _, err := sourceOf(t, h, id); !errors.Is(err, ErrSourceNotFound) {
			t.Errorf("%s: err %v, want ErrSourceNotFound", name, err)
		}
	}
}

func TestSourceOfAnUnknownRunIsTheSourcesError(t *testing.T) {
	t.Parallel()
	src := source(contract.PhaseSucceeded, terminated())
	src.SetRunError(kube.ErrRunNotFound)
	h := newHub(t, src)
	id := contract.EventID(attemptID, streamtest.Origin("source"), 1, 0)
	if _, err := sourceOf(t, h, id); !errors.Is(err, kube.ErrRunNotFound) {
		t.Errorf("err %v, want kube.ErrRunNotFound", err)
	}
}

func TestSourcesAreBoundedOldestFirst(t *testing.T) {
	t.Parallel()
	const kept = 3
	b := sources{bounds: Bounds{EventsPerRun: kept, SourceBytes: contract.MaxRunSourceBytes}}
	origin := streamtest.Origin("bounded")
	for n := int64(1); n <= kept+2; n++ {
		b.add(attemptID, origin, streamtest.Line(n, `{"n":1}`))
	}
	for n := int64(1); n <= kept+2; n++ {
		held := b.find(refOf(t, contract.EventID(attemptID, origin, n, 0))) != nil
		if want := n > 2; held != want {
			t.Errorf("line %d held %v, want %v", n, held, want)
		}
	}

	small := sources{bounds: Bounds{EventsPerRun: contract.MaxEventsPerRun, SourceBytes: 10}}
	small.add(attemptID, origin, streamtest.Line(1, "12345678"))
	small.add(attemptID, origin, streamtest.Line(2, "abcdefgh"))
	if small.find(refOf(t, contract.EventID(attemptID, origin, 1, 0))) != nil || small.bytes != int64(len("abcdefgh")) {
		t.Errorf("past SourceBytes: line 1 still held, %d bytes", small.bytes)
	}
}

func TestSourceTextIsCutAtARuneBoundary(t *testing.T) {
	t.Parallel()
	b := sources{bounds: Bounds{}.withDefaults()}
	origin := streamtest.Origin("long")
	// A two-byte rune straddles the limit.
	text := strings.Repeat("a", contract.MaxSourceBytes-1) + "é" + strings.Repeat("b", contract.MaxSourceBytes)
	b.add(attemptID, origin, streamtest.Line(1, text))
	l := b.find(refOf(t, contract.EventID(attemptID, origin, 1, 0)))
	if l == nil {
		t.Fatal("line not held")
	}
	if want := strings.Repeat("a", contract.MaxSourceBytes-1); l.text != want || l.bytes != int64(len(text)) {
		t.Errorf("text of %d bytes, bytes %d; want %d and %d", len(l.text), l.bytes, len(want), len(text))
	}
}
