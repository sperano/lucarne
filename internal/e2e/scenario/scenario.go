// Package scenario is the fake cluster Lucarne's end-to-end tests read
// through the real source, stream and server: one finished run per runner
// fixture, runs with hostile, oversized and long output, runs Lucarne must
// never read, and live runs a test drives. Every name, UID and line is
// fabricated: the repository is public.
package scenario

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/fixtures"
	"github.com/sperano/lucarne/internal/kube"
	"github.com/sperano/lucarne/internal/kube/kubetest"
)

const (
	// Cluster is the cluster on every run key.
	Cluster = "example"
	// Namespace is the namespace the server may read.
	Namespace = "agents"
	// PrivateNamespace holds a run the server must never read: it is not
	// allowlisted.
	PrivateNamespace = "private"
)

// Names of the runs that are not fixtures.
const (
	Hostile        = "hostile"
	LongLine       = "long-line"
	LongSession    = "long-session"
	ForeignPod     = "foreign-pod"
	PodDeleted     = "pod-deleted"
	UnknownRunner  = "unknown-runner"
	PrivateRun     = "private"
	fixturePrefix  = "fixture/"
	livePrefix     = "live-"
	fixtureLogGlob = "*/*/*.log"
)

// failedFixtures are the fixture scenarios whose run failed: the agent
// exited 1 and the Task is Failed.
var failedFixtures = map[string]bool{
	"failed-session": true,
	"limit-hit":      true,
	"errors":         true,
	"interrupted":    true,
}

// failedExitCode is the exit code of a failed agent.
const failedExitCode = 1

// Run is a run of the world.
type Run struct {
	Name string
	Key  contract.RunKey
	// Runner is the Kelos agent type.
	Runner string
	Task   *kubetest.Run
	// Pod is the run's pod, nil when it has none any more.
	Pod *kubetest.Pod
}

// Link is the run's deep link.
func (r *Run) Link() string { return r.Key.LinkPath() }

// World is the fake cluster and its runs. It is safe for concurrent use.
type World struct {
	Cluster *kubetest.Cluster
	mu      sync.Mutex
	runs    map[string]*Run
	lives   int
}

// New builds the world.
func New() (*World, error) {
	w := &World{Cluster: kubetest.New(Cluster), runs: map[string]*Run{}}
	steps := []func() error{w.addFixtures, w.addHostile, w.addLong, w.addUnread, w.addOthers}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// Source reads the world's allowlisted namespace.
func (w *World) Source() (*kube.Source, error) { return w.Cluster.Source(Namespace) }

// Run is the run called name, nil when there is none.
func (w *World) Run(name string) *Run {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.runs[name]
}

// Names are the names of the runs, sorted.
func (w *World) Names() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	names := make([]string, 0, len(w.runs))
	for name := range w.runs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FixtureName is the name of the run of a fixture file,
// <runner>/<version>/<scenario>.log.
func FixtureName(file string) string {
	runner := strings.SplitN(file, "/", 2)[0]
	return fixturePrefix + runner + "/" + strings.TrimSuffix(path.Base(file), ".log")
}

// FixtureFailed reports whether the run of a fixture file failed.
func FixtureFailed(file string) bool {
	return failedFixtures[strings.TrimSuffix(path.Base(file), ".log")]
}

// FixtureFiles are the fixture files, <runner>/<version>/<scenario>.log.
func FixtureFiles() ([]string, error) {
	files, err := fs.Glob(fixtures.Files, fixtureLogGlob)
	if err != nil {
		return nil, fmt.Errorf("scenario: list fixtures: %w", err)
	}
	return files, nil
}

// AddLive adds a running Claude Code run with an empty log, for a test to
// drive, and returns it.
func (w *World) AddLive() (*Run, error) {
	w.mu.Lock()
	w.lives++
	name := fmt.Sprintf("%s%d", livePrefix, w.lives)
	w.mu.Unlock()
	return w.add(name, Namespace, string(contract.RunnerClaudeCode), kubetest.PhaseRunning)
}

// Finish ends a run's agent with exitCode and its Task with phase.
func (r *Run) Finish(exitCode int32, phase string) error {
	if err := r.Pod.Terminate(exitCode); err != nil {
		return err
	}
	return r.Task.SetPhase(phase)
}

func (w *World) addFixtures() error {
	files, err := FixtureFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		data, err := fs.ReadFile(fixtures.Files, file)
		if err != nil {
			return fmt.Errorf("scenario: read %s: %w", file, err)
		}
		runner := strings.SplitN(file, "/", 2)[0]
		if err := w.addFinished(FixtureName(file), runner, LogLines(string(data)), FixtureFailed(file)); err != nil {
			return err
		}
	}
	return nil
}

// addFinished adds a run in Namespace whose agent wrote lines and exited.
func (w *World) addFinished(name, runner string, lines []string, failed bool) error {
	r, err := w.add(name, Namespace, runner, kubetest.PhaseRunning)
	if err != nil {
		return err
	}
	r.Pod.Write(lines...)
	if failed {
		return r.Finish(failedExitCode, kubetest.PhaseFailed)
	}
	return r.Finish(0, kubetest.PhaseSucceeded)
}

// add creates a Task called after name with one running pod.
func (w *World) add(name, namespace, runner, phase string) (*Run, error) {
	task, err := w.Cluster.AddTask(kubetest.TaskSpec{
		Namespace: namespace, Name: taskName(name), UID: newUID(), Agent: runner, Phase: phase,
	})
	if err != nil {
		return nil, err
	}
	pod, err := task.AddPod(taskName(name)+"-abcde", newUID())
	if err != nil {
		return nil, err
	}
	r := &Run{Name: name, Key: task.Key(), Runner: runner, Task: task, Pod: pod}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, taken := w.runs[name]; taken {
		return nil, fmt.Errorf("scenario: two runs called %s", name)
	}
	w.runs[name] = r
	return r, nil
}

// LogLines splits a log into its lines, without the newline after the last.
func LogLines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// taskName is a Kubernetes name made from a run's name.
func taskName(name string) string {
	return "example-" + strings.NewReplacer("/", "-", ".", "-").Replace(name)
}

// newUID is a random version 4 UUID, as Kubernetes gives objects.
func newUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])  // crypto/rand.Read never fails
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
