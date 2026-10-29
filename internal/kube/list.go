package kube

import (
	"cmp"
	"context"
	"maps"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sperano/lucarne/contract"
)

// ListRuns lists the Tasks of every allowlisted namespace as runs, newest
// first, for the UI to search: at most contract.MaxListedRuns, the older
// ones counted in OmittedRuns. A Task that does not decode is left out, as
// its link could not open it either. Each namespace is listed in pages, and
// at most twice the limit is held between pages, so a crowded namespace
// does not grow the list without bound.
func (s *Source) ListRuns(ctx context.Context) (contract.RunList, error) {
	list := contract.RunList{Schema: contract.SchemaVersion}
	for _, namespace := range slices.Sorted(maps.Keys(s.namespaces)) {
		if err := s.listNamespace(ctx, namespace, &list); err != nil {
			return contract.RunList{}, err
		}
	}
	keepNewest(&list, contract.MaxListedRuns)
	return list, nil
}

// listNamespace adds the runs of namespace's Tasks to list, page by page.
func (s *Source) listNamespace(ctx context.Context, namespace string, list *contract.RunList) error {
	opts := metav1.ListOptions{Limit: taskListPageSize}
	for {
		page, err := s.clients.Dynamic.Resource(TaskResource).Namespace(namespace).List(ctx, opts)
		if err != nil {
			return classify(ctx, err, "list tasks")
		}
		for i := range page.Items {
			t, err := decodeTask(&page.Items[i])
			if err != nil {
				continue
			}
			list.Runs = append(list.Runs, s.newRun(namespace, t).Run)
		}
		if len(list.Runs) > listCompactFactor*contract.MaxListedRuns {
			keepNewest(list, contract.MaxListedRuns)
		}
		if opts.Continue = page.GetContinue(); opts.Continue == "" {
			return nil
		}
	}
}

// listCompactFactor is how many times the limit the runs listed may grow
// to before the oldest are dropped: compacting less often sorts less.
const listCompactFactor = 2

// keepNewest sorts list's runs newest first and drops all but limit of
// them, counting the dropped ones in OmittedRuns.
func keepNewest(list *contract.RunList, limit int) {
	slices.SortFunc(list.Runs, newestFirst)
	if len(list.Runs) > limit {
		list.OmittedRuns += int64(len(list.Runs) - limit)
		list.Runs = slices.Clip(list.Runs[:limit])
	}
}

// newestFirst orders runs by creation time, newest first, a run without
// one last; then by namespace, Task name and UID, so that the order is
// stable across requests.
func newestFirst(a, b contract.Run) int {
	if c := cmpCreated(a.Created, b.Created); c != 0 {
		return c
	}
	return cmp.Or(
		cmp.Compare(a.Key.Namespace, b.Key.Namespace),
		cmp.Compare(a.TaskName, b.TaskName),
		cmp.Compare(a.Key.TaskUID, b.Key.TaskUID),
	)
}

// cmpCreated orders a later time first and a missing one last.
func cmpCreated(a, b *contract.Timestamp) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	default:
		return b.Compare(a.Time)
	}
}
