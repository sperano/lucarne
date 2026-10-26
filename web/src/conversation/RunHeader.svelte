<script lang="ts">
  import type { Attempt, Run, Stream } from '../lib/contract';
  import { formatCount, formatTime } from '../lib/format';
  import {
    CONTAINER_STATE_LABELS,
    COVERAGE_LABELS,
    RUN_PHASE_LABELS,
    RUNNER_LABELS,
  } from '../lib/labels';
  import AttemptPicker from './AttemptPicker.svelte';
  import Completeness from './Completeness.svelte';

  let {
    run,
    attempts,
    selected,
    stream,
    onselect,
  }: {
    run: Run;
    attempts: readonly Attempt[];
    selected: Attempt | undefined;
    stream: Stream;
    onselect: (attemptId: string) => void;
  } = $props();

  const runnerLabel = $derived.by(() => {
    const kind = selected?.runner.kind ?? run.runner;
    if (kind === undefined) return undefined;
    const version = selected?.runner.version;
    return version === undefined ? RUNNER_LABELS[kind] : `${RUNNER_LABELS[kind]} ${version}`;
  });
  const containerLabel = $derived.by(() => {
    if (!selected) return '';
    const c = selected.container;
    const exit = c.exitCode === undefined ? '' : `, exit code ${String(c.exitCode)}`;
    const reason = c.reason === undefined ? '' : ` (${c.reason})`;
    return `${CONTAINER_STATE_LABELS[c.state]}${exit}${reason}`;
  });
  const coverageLabel = $derived.by(() => {
    if (!selected) return '';
    const c = selected.coverage;
    const reason = c.reason === undefined ? '' : ` (${c.reason})`;
    return `${COVERAGE_LABELS[c.state]}${reason}, ${formatCount(c.lines)} lines read`;
  });
  // The Task's lifecycle times the source gave, in order.
  const times = $derived(
    (
      [
        ['Created', run.created],
        ['Started', run.started],
        ['Completed', run.completed],
      ] as const
    ).flatMap(([label, time]) => (time === undefined ? [] : [{ label, time }])),
  );
</script>

<header class="run">
  <div class="title">
    <h2>{run.taskName}</h2>
    <span class="phase {run.phase}">{RUN_PHASE_LABELS[run.phase]}</span>
    {#if run.deleted}<span class="phase failed">Task deleted</span>{/if}
  </div>
  <dl class="facts">
    {#if runnerLabel !== undefined}
      <dt>Runner</dt>
      <dd>{runnerLabel}</dd>
    {/if}
    <dt>Task</dt>
    <dd><code>{run.key.cluster}/{run.key.namespace}</code> · UID <code>{run.key.taskUID}</code></dd>
    {#each times as { label, time } (label)}
      <dt>{label}</dt>
      <dd>{formatTime(time)}</dd>
    {/each}
    {#if selected}
      <dt>Pod</dt>
      <dd>
        <code>{selected.podName}</code>, container <code>{selected.key.container}</code>, restart
        {selected.key.restart}: {containerLabel}
      </dd>
      <dt>Log</dt>
      <dd>{coverageLabel}</dd>
    {/if}
  </dl>
  {#if attempts.length > 1}
    <AttemptPicker {attempts} {selected} {onselect} />
  {/if}
  {#if selected}
    <Completeness attempt={selected} />
  {/if}
  {#if stream.droppedEvents > 0}
    <p class="notice warning" role="note">
      The {formatCount(stream.droppedEvents)} oldest events of this run were dropped to keep it within
      the server's bounds; the transcript starts after them.
    </p>
  {/if}
</header>

<style>
  .run {
    margin: 0 0 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .title {
    display: flex;
    gap: 0.75rem;
    align-items: baseline;
    flex-wrap: wrap;
  }
  h2 {
    margin: 0;
    font-size: 1.25rem;
    overflow-wrap: anywhere;
  }
  .phase {
    font-size: 0.8125rem;
    font-weight: 600;
    padding: 0 0.5rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    color: var(--text-muted);
  }
  .phase.succeeded {
    color: var(--ok);
  }
  .phase.failed {
    color: var(--error);
  }
  .facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.125rem 1rem;
    margin: 0;
    font-size: 0.875rem;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
