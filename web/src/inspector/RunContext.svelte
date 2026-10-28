<script lang="ts">
  import AttemptPicker from '../conversation/AttemptPicker.svelte';
  import Completeness from '../conversation/Completeness.svelte';
  import type { Attempt, Run, Stream, Time, TranscriptEvent } from '../lib/contract';
  import { formatCount, formatTime } from '../lib/format';
  import { sessionFacts } from '../lib/inspector';
  import {
    CONTAINER_STATE_LABELS,
    COVERAGE_LABELS,
    RUN_PHASE_LABELS,
    RUNNER_LABELS,
  } from '../lib/labels';
  import { usageParts } from '../lib/usage';

  let {
    run,
    attempts,
    selected,
    stream,
    events,
    onselect,
  }: {
    run: Run;
    attempts: readonly Attempt[];
    selected: Attempt | undefined;
    stream: Stream;
    /** The events of the selected attempt the page holds. */
    events: readonly TranscriptEvent[];
    onselect: (attemptId: string) => void;
  } = $props();

  const facts = $derived(sessionFacts(events));
  const runner = $derived.by(() => {
    const kind = selected?.runner.kind ?? run.runner;
    if (kind === undefined) return undefined;
    const version = selected?.runner.version ?? facts.runnerVersion;
    return version === undefined ? RUNNER_LABELS[kind] : `${RUNNER_LABELS[kind]} ${version}`;
  });
  const container = $derived.by(() => {
    if (!selected) return undefined;
    const c = selected.container;
    const exit = c.exitCode === undefined ? '' : `, exit code ${String(c.exitCode)}`;
    const reason = c.reason === undefined ? '' : ` (${c.reason})`;
    return `${CONTAINER_STATE_LABELS[c.state]}${exit}${reason}`;
  });
  const usage = $derived(facts.usage ? usageParts(facts.usage) : undefined);
  const runTimes: readonly { label: string; time: Time | undefined }[] = $derived([
    { label: 'Created', time: run.created },
    { label: 'Started', time: run.started },
    { label: 'Completed', time: run.completed },
  ]);
</script>

{#snippet fact(label: string, value: string | undefined)}
  <dt>{label}</dt>
  {#if value === undefined}
    <dd class="unknown">Unknown</dd>
  {:else}
    <dd>{value}</dd>
  {/if}
{/snippet}

{#snippet time(label: string, value: Time | undefined)}
  {@render fact(label, value === undefined ? undefined : formatTime(value))}
{/snippet}

<div class="context">
  <div class="title">
    <h2>{run.taskName}</h2>
    <span class="phase {run.phase}">{RUN_PHASE_LABELS[run.phase]}</span>
    {#if run.deleted}<span class="phase failed">Task deleted</span>{/if}
  </div>
  {#if attempts.length > 1}
    <AttemptPicker {attempts} {selected} {onselect} />
  {/if}

  <h3>Run</h3>
  <dl class="facts">
    <dt>Task</dt>
    <dd>
      <code>{run.key.cluster}/{run.key.namespace}</code><br />UID <code>{run.key.taskUID}</code>
    </dd>
    {@render fact('Runner', runner)}
    {@render fact('Model', facts.model)}
    {@render fact('Session', facts.sessionId)}
    {#each runTimes as t (t.label)}{@render time(t.label, t.time)}{/each}
  </dl>

  {#if selected}
    <h3>Attempt</h3>
    <dl class="facts">
      <dt>Pod</dt>
      <dd><code>{selected.podName}</code></dd>
      <dt>Container</dt>
      <dd><code>{selected.key.container}</code>, restart {selected.key.restart}</dd>
      {@render fact('State', container)}
      {@render time('Started', selected.container.startedAt)}
      {@render time('Finished', selected.container.finishedAt)}
      <dt>Log</dt>
      <dd>
        {COVERAGE_LABELS[selected.coverage.state]}{selected.coverage.reason === undefined
          ? ''
          : ` (${selected.coverage.reason})`}, {formatCount(selected.coverage.lines)} lines read
      </dd>
      <dt>Events</dt>
      <dd>{formatCount(events.length)} held</dd>
    </dl>

    <h3>Usage</h3>
    {#if usage === undefined}
      <p class="unknown">
        Unknown: the log reports no session total{facts.partialUsages > 0
          ? `, only ${formatCount(facts.partialUsages)} usage events of smaller scopes (see the list)`
          : ''}.
      </p>
    {:else if usage.length === 0}
      <p class="unknown">The session total reports no counter.</p>
    {:else}
      <ul class="usage" aria-label="Session usage">
        {#each usage as part, i (i)}<li>{part}</li>{/each}
      </ul>
    {/if}
    <Completeness attempt={selected} />
  {/if}
  {#if stream.droppedEvents > 0}
    <p class="notice warning" role="note">
      The {formatCount(stream.droppedEvents)} oldest events of this run were dropped to keep it within
      the server's bounds; the list starts after them.
    </p>
  {/if}
</div>

<style>
  .context {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    font-size: 0.875rem;
  }
  .title {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    flex-wrap: wrap;
  }
  h2 {
    margin: 0;
    font-size: 1.125rem;
    overflow-wrap: anywhere;
  }
  h3 {
    margin: 0.5rem 0 0;
    font-size: 0.8125rem;
    text-transform: uppercase;
    color: var(--text-muted);
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
    gap: 0.125rem 0.75rem;
    margin: 0;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .unknown {
    color: var(--text-muted);
    font-style: italic;
  }
  p {
    margin: 0;
  }
  .usage {
    margin: 0;
    padding-left: 1.25rem;
  }
</style>
