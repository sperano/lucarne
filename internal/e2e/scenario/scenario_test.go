package scenario

import (
	"context"
	"testing"
)

func TestNewBuildsEveryRun(t *testing.T) {
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	files, err := FixtureFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixture found")
	}
	for _, file := range files {
		if w.Run(FixtureName(file)) == nil {
			t.Errorf("no run of %s", file)
		}
	}
	src, err := w.Source()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{Hostile, LongLine, LongSession, ForeignPod, PodDeleted, UnknownRunner, Retried} {
		r := w.Run(name)
		if r == nil {
			t.Fatalf("no run %s", name)
		}
		if _, err := src.Run(context.Background(), r.Key); err != nil {
			t.Errorf("run %s: %v", name, err)
		}
	}
	if _, err := src.Run(context.Background(), w.Run(PrivateRun).Key); err == nil {
		t.Error("the private run is readable")
	}
}

func TestAddLiveNamesEachRun(t *testing.T) {
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a, err := w.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.AddLive()
	if err != nil {
		t.Fatal(err)
	}
	if a.Name == b.Name || a.Key == b.Key {
		t.Errorf("live runs %s and %s share a name or key", a.Name, b.Name)
	}
	src, err := w.Source()
	if err != nil {
		t.Fatal(err)
	}
	run, err := src.Run(context.Background(), a.Key)
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := src.Attempts(context.Background(), run)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts %v, %v", attempts, err)
	}
}

func TestFixtureName(t *testing.T) {
	if got := FixtureName("claude-code/2.1.296/session-basic.log"); got != "fixture/claude-code/session-basic" {
		t.Errorf("FixtureName = %q", got)
	}
	if !FixtureFailed("codex/0.162.1/failed-session.log") || FixtureFailed("codex/0.162.1/session-basic.log") {
		t.Error("FixtureFailed misreads the scenarios")
	}
}
