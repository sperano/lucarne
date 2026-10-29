// Searching the run list of the home page: a run matches when every word of
// the query appears, ignoring case, in its Task name, namespace, Task UID,
// phase or runner. Only the runs the server listed are searched.
import type { Run } from './contract';
import { RUN_PHASE_LABELS, RUNNER_LABELS } from './labels';

/** How many matching runs the home page shows; refining the search reaches the others. */
export const SHOWN_RUNS = 100;

// Runs never change (a reload replaces the list), so their text is computed once.
const texts = new WeakMap<Run, string>();

/** The runs, in their order, that match every word of query; all of them for a blank query. */
export function matchRuns(runs: readonly Run[], query: string): Run[] {
  const words = query
    .toLowerCase()
    .split(/\s+/)
    .filter((word) => word !== '');
  if (words.length === 0) return [...runs];
  return runs.filter((run) => {
    const text = textOf(run);
    return words.every((word) => text.includes(word));
  });
}

function textOf(run: Run): string {
  let text = texts.get(run);
  if (text === undefined) {
    const runner = run.runner === undefined ? [] : [run.runner, RUNNER_LABELS[run.runner]];
    text = [
      run.taskName,
      run.key.namespace,
      run.key.taskUID,
      RUN_PHASE_LABELS[run.phase],
      ...runner,
    ]
      .join('\n')
      .toLowerCase();
    texts.set(run, text);
  }
  return text;
}
