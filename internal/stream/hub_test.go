package stream

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/stream/streamtest"
)

// otherKey is a run the fabricated source does not know.
var otherKey = contract.RunKey{Cluster: streamtest.Cluster, Namespace: streamtest.Namespace, TaskUID: streamtest.RetryPodUID}

func TestNewNeedsASource(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Error("a hub without source was made")
	}
}

func TestRunsThatCannotBeOpenedAreNotKept(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		key      contract.RunKey
		runErr   error
		listErr  error
		want     error
		wantRuns int
	}{
		"unknown run":       {key: otherKey, want: kube.ErrRunNotFound},
		"source down":       {key: streamtest.Key, runErr: fmt.Errorf("%w: timeout", kube.ErrUnavailable), want: kube.ErrUnavailable},
		"forbidden":         {key: streamtest.Key, runErr: fmt.Errorf("%w: rbac", kube.ErrForbidden), want: kube.ErrForbidden},
		"attempts not read": {key: streamtest.Key, listErr: fmt.Errorf("%w: timeout", kube.ErrUnavailable), want: kube.ErrUnavailable},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := source(contract.PhaseRunning)
			src.SetRunError(tt.runErr)
			src.SetAttemptsError(tt.listErr)
			h := newHub(t, src)
			for range 2 {
				if _, err := h.Snapshot(context.Background(), tt.key); !errors.Is(err, tt.want) {
					t.Fatalf("snapshot error %v, want %v", err, tt.want)
				}
				if _, err := h.Subscribe(context.Background(), tt.key, ""); !errors.Is(err, tt.want) {
					t.Fatalf("subscribe error %v, want %v", err, tt.want)
				}
			}
			if calls := src.RunCalls(); calls != 4 {
				t.Errorf("%d resolutions, want one per request", calls)
			}
		})
	}
}

func TestConcurrentFirstRequestsResolveOnce(t *testing.T) {
	t.Parallel()
	const requests = 8
	src := source(contract.PhaseRunning, running())
	h := newHub(t, src)
	var wg sync.WaitGroup
	epochs := make([]string, requests)
	for i := range requests {
		wg.Go(func() { epochs[i] = snapshotOf(t, h).Stream.Epoch })
	}
	wg.Wait()
	for _, epoch := range epochs {
		if epoch != epochs[0] {
			t.Fatalf("epochs %q, want one history", epochs)
		}
	}
	if calls := src.RunCalls(); calls != 1 {
		t.Errorf("%d resolutions, want 1", calls)
	}
}

func TestServerRestartChangesTheEpoch(t *testing.T) {
	t.Parallel()
	src, lines := live(t)
	before := newHub(t, src)
	snapshotOf(t, before)
	send(lines, 1, 2)
	snap := waitEvents(t, before, 2)
	before.Close()
	after := newHub(t, source(contract.PhaseRunning, running()))
	sub := subscribe(t, after, cursorOf(snap))
	if sub.Reset == nil {
		t.Fatal("a cursor of the stopped server resumed")
	}
	if reset := decode[contract.Reset](t, sub.Reset.Data); reset.Reason != contract.ResetEpochChanged || reset.Epoch == snap.Stream.Epoch {
		t.Errorf("reset %+v, want epoch_changed to a new epoch", reset)
	}
}

func TestCloseEndsStreamsWithoutEnd(t *testing.T) {
	t.Parallel()
	src, _ := live(t)
	h := newHub(t, src)
	sub := subscribe(t, h, cursorOf(snapshotOf(t, h)))
	h.Close()
	select {
	case <-sub.Done():
	case <-time.After(testTimeout):
		t.Fatal("stream still open after Close")
	}
	if sub.Err() != nil {
		t.Errorf("closed with %s, want no reset", sub.Err().Data)
	}
	if _, err := h.Snapshot(context.Background(), streamtest.Key); !errors.Is(err, ErrClosed) {
		t.Errorf("snapshot after Close: %v, want ErrClosed", err)
	}
}

func TestIdleRunIsDropped(t *testing.T) {
	t.Parallel()
	const idle = 20 * time.Millisecond
	h := newHub(t, source(contract.PhaseRunning, running()), func(c *Config) { c.IdleTimeout = idle })
	var sub *Subscription
	eventually(t, "a live subscription", func() bool {
		// Under load the run can go idle between the snapshot and the
		// subscription, which then resets to the reopened run's epoch.
		sub = subscribe(t, h, cursorOf(snapshotOf(t, h)))
		return sub.Reset == nil
	})
	time.Sleep(2 * idle)
	if h.followed(streamtest.Key) == nil {
		t.Fatal("a watched run was dropped")
	}
	sub.Close()
	eventually(t, "the idle run to be dropped", func() bool { return h.followed(streamtest.Key) == nil })
}

func TestFullHubDropsTheRunIdleLongest(t *testing.T) {
	t.Parallel()
	h := newHub(t, source(contract.PhaseRunning, running()), func(c *Config) { c.MaxRuns = 1 })
	sub := subscribe(t, h, cursorOf(snapshotOf(t, h)))
	if _, err := h.Snapshot(context.Background(), otherKey); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second run while the first is watched: %v, want ErrBusy", err)
	}
	sub.Close()
	if _, err := h.Snapshot(context.Background(), otherKey); !errors.Is(err, kube.ErrRunNotFound) {
		t.Fatalf("a second run once the first is idle: %v, want it resolved", err)
	}
	if h.followed(streamtest.Key) != nil {
		t.Error("the idle run was kept")
	}
}

func TestStreamAfterAnotherEpochResets(t *testing.T) {
	t.Parallel()
	h := newHub(t, source(contract.PhaseRunning, running()), func(c *Config) { c.IdleTimeout = 10 * time.Millisecond })
	old := snapshotOf(t, h)
	eventually(t, "the idle run to be dropped", func() bool { return h.followed(streamtest.Key) == nil })
	sub := subscribe(t, h, cursorOf(old))
	if sub.Reset == nil || decode[contract.Reset](t, sub.Reset.Data).Reason != contract.ResetEpochChanged {
		t.Errorf("subscription %+v, want an epoch_changed reset", sub)
	}
}
