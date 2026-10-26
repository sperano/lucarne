package kube

import (
	"context"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/sperano/lucarne/contract"
)

// TaskResource is the Kelos Task resource, kelos.dev/v1alpha2 in Kelos 0.60.0.
var TaskResource = schema.GroupVersionResource{Group: "kelos.dev", Version: "v1alpha2", Resource: "tasks"}

const (
	// taskListPageSize is how many Tasks one list request asks for; a
	// namespace holds the Tasks of the last 72 hours.
	taskListPageSize = 250
	// defaultRetryDelay is the wait between two tries to follow a log:
	// a reconnect after the stream broke, a poll of a container that has
	// not started.
	defaultRetryDelay = 2 * time.Second
)

// Errors of run resolution. Each one returned wraps one of them.
var (
	// ErrRunNotFound is a run key with no Task: another cluster, a
	// namespace not allowlisted, or no Task with that UID (any more).
	ErrRunNotFound = errors.New("run not found")
	// ErrRunDeleted is a run whose Task was found once and is gone, or
	// whose name now holds another Task.
	ErrRunDeleted = errors.New("run deleted")
	// ErrAttemptNotFound is an attempt key that names no instance of the
	// run's agent container.
	ErrAttemptNotFound = errors.New("attempt not found")
	// ErrForbidden is a request the Kubernetes API refused to the server's
	// credentials.
	ErrForbidden = errors.New("forbidden by the Kubernetes API")
	// ErrUnavailable is a Kubernetes API that did not answer, or answered
	// with a server-side failure.
	ErrUnavailable = errors.New("unavailable: the Kubernetes API did not answer")
)

// Config says which runs the source may read.
type Config struct {
	// Cluster is the name of the cluster on run keys; Kubernetes has none.
	Cluster string
	// Namespaces is the allowlist: no other namespace is ever read.
	Namespaces []string
	// RetryDelay is the wait before a follow retries; zero means 2s.
	RetryDelay time.Duration
}

// Clients are the Kubernetes clients the source reads with: Dynamic for
// Tasks, Kube for Jobs and pods, Logs for container logs.
type Clients struct {
	Dynamic dynamic.Interface
	Kube    kubernetes.Interface
	Logs    LogStreamer
}

// Source reads the runs of one cluster's allowlisted namespaces. It is safe
// for concurrent use.
type Source struct {
	cluster    string
	namespaces map[string]bool
	clients    Clients
	// retryDelay is the wait before a follow retries; a test sets it to 0.
	retryDelay time.Duration
}

// New returns a source reading with clients what cfg allows.
func New(cfg Config, clients Clients) (*Source, error) {
	if len(cfg.Namespaces) == 0 {
		return nil, errors.New("kube: no namespace allowlisted")
	}
	namespaces := make(map[string]bool, len(cfg.Namespaces))
	for _, ns := range cfg.Namespaces {
		// A key of the cluster, the namespace and any UID validates both.
		if err := (contract.RunKey{Cluster: cfg.Cluster, Namespace: ns, TaskUID: zeroUID}).Validate(); err != nil {
			return nil, fmt.Errorf("kube: config: %w", err)
		}
		namespaces[ns] = true
	}
	if clients.Dynamic == nil || clients.Kube == nil || clients.Logs == nil {
		return nil, errors.New("kube: a client is missing")
	}
	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultRetryDelay
	}
	return &Source{cluster: cfg.Cluster, namespaces: namespaces, clients: clients, retryDelay: retryDelay}, nil
}

// zeroUID is a valid UID, used to validate a configured cluster and
// namespace with contract.RunKey.Validate.
const zeroUID = "00000000-0000-0000-0000-000000000000"

// Run is a resolved run: the Task as last read, keyed by UID.
type Run struct {
	contract.Run
	// jobName is the Task's Job: status.jobName, or the Task's name
	// before the status names one (Kelos names the Job after its Task).
	jobName string
}

// Run resolves key to its Task. A key outside the configured cluster and
// allowlist is ErrRunNotFound before any API call.
func (s *Source) Run(ctx context.Context, key contract.RunKey) (Run, error) {
	if err := key.Validate(); err != nil {
		return Run{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
	}
	if err := s.readable(key); err != nil {
		return Run{}, err
	}
	task, err := s.findTask(ctx, key.Namespace, key.TaskUID)
	if err != nil {
		return Run{}, err
	}
	return s.newRun(key.Namespace, task), nil
}

// readable is ErrRunNotFound for a key of another cluster or of a namespace
// not allowlisted. Every entry point checks it, a Run's key included, which
// its holder could change.
func (s *Source) readable(key contract.RunKey) error {
	if key.Cluster != s.cluster || !s.namespaces[key.Namespace] {
		return fmt.Errorf("%w: %s is not readable here", ErrRunNotFound, key.LinkPath())
	}
	return nil
}

// Refresh reads run's Task again by name, and ErrRunDeleted when it is
// gone or the name now holds another Task.
func (s *Source) Refresh(ctx context.Context, run Run) (Run, error) {
	if err := s.readable(run.Key); err != nil {
		return Run{}, err
	}
	task, err := s.taskByName(ctx, run.Key.Namespace, run.TaskName, run.Key.TaskUID)
	if err != nil {
		return Run{}, err
	}
	return s.newRun(run.Key.Namespace, task), nil
}

func (s *Source) newRun(namespace string, t task) Run {
	key := contract.RunKey{Cluster: s.cluster, Namespace: namespace, TaskUID: t.UID}
	return Run{Run: t.contractRun(key), jobName: t.jobName()}
}

// findTask lists the namespace's Tasks, page by page, for the one with uid:
// the API gets objects by name only.
func (s *Source) findTask(ctx context.Context, namespace, uid string) (task, error) {
	opts := metav1.ListOptions{Limit: taskListPageSize}
	for {
		list, err := s.clients.Dynamic.Resource(TaskResource).Namespace(namespace).List(ctx, opts)
		if err != nil {
			return task{}, classify(ctx, err, "list tasks")
		}
		for i := range list.Items {
			if string(list.Items[i].GetUID()) == uid {
				return decodeTask(&list.Items[i])
			}
		}
		if opts.Continue = list.GetContinue(); opts.Continue == "" {
			return task{}, fmt.Errorf("%w: no task %s in namespace %s", ErrRunNotFound, uid, namespace)
		}
	}
}

// taskByName gets the Task called name, which must still be the one with uid.
func (s *Source) taskByName(ctx context.Context, namespace, name, uid string) (task, error) {
	obj, err := s.clients.Dynamic.Resource(TaskResource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return task{}, fmt.Errorf("%w: task %s is gone", ErrRunDeleted, name)
		}
		return task{}, classify(ctx, err, "get task")
	}
	if string(obj.GetUID()) != uid {
		return task{}, fmt.Errorf("%w: task name %s now holds another task", ErrRunDeleted, name)
	}
	return decodeTask(obj)
}
