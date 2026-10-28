<script lang="ts">
  import type { Attempt, RunKey, TranscriptEvent } from '../lib/contract';
  import { formatCount, formatTime } from '../lib/format';
  import { COVERAGE_LABELS, STATUS_LABELS } from '../lib/labels';
  import { runHref } from '../lib/route';
  import CopyButton from './CopyButton.svelte';

  let {
    event,
    attempt,
    runKey,
  }: {
    event: TranscriptEvent;
    /** The attempt the event belongs to. */
    attempt: Attempt;
    runKey: RunKey;
  } = $props();

  const source = $derived(event.source);
  const origin = $derived(event.id.split(':')[1] ?? '');
  const coverage = $derived.by(() => {
    const c = attempt.coverage;
    const reason = c.reason === undefined ? '' : ` (${c.reason})`;
    return `${COVERAGE_LABELS[c.state]}${reason}: ${count(c.lines, 'line')} read, ${count(c.gaps, 'gap')}, ${count(c.truncatedLines, 'line')} cut`;
  });
  const diff = $derived.by(() => {
    if (event.kind !== 'edit') return undefined;
    const e = event.edit;
    const given = [
      e.diff === undefined ? '' : 'a diff',
      e.before === undefined ? '' : 'the text before',
      e.after === undefined ? '' : 'the text after',
    ].filter((g) => g !== '');
    return given.length === 0 ? undefined : given.join(', ');
  });

  function count(n: number, noun: string): string {
    return `${formatCount(n)} ${noun}${n === 1 ? '' : 's'}`;
  }

  function link(): string {
    const href = runHref(runKey, { attempt: event.attempt, view: 'inspector', event: event.id });
    return new URL(href, window.location.origin).href;
  }
</script>

{#snippet fact(label: string, value: string | undefined, unknown: string)}
  <dt>{label}</dt>
  {#if value === undefined}
    <dd class="unknown">{unknown}</dd>
  {:else}
    <dd>{value}</dd>
  {/if}
{/snippet}

{#snippet time(label: string, value: string | undefined)}
  <dt>{label}</dt>
  {#if value === undefined}
    <dd class="unknown">Not in the log</dd>
  {:else}
    <dd><time datetime={value}>{formatTime(value)}</time></dd>
  {/if}
{/snippet}

<dl class="facts">
  <dt>Event</dt>
  <dd>
    <code>{event.id}</code>
    <span class="copies">
      <CopyButton label="Copy the ID" text={() => event.id} />
      <CopyButton label="Copy the link" text={link} />
    </span>
  </dd>
  <dt>Attempt</dt>
  <dd>
    Pod <code>{attempt.podName}</code>, container <code>{attempt.key.container}</code>, restart {attempt
      .key.restart}
  </dd>
  <dt>Log</dt>
  <dd>{coverage}</dd>
  <dt>Line</dt>
  <dd>
    {#if event.kind === 'gap'}
      Before line {formatCount(source.line)}, gap {source.part + 1} there
    {:else}
      {formatCount(source.line)}{source.part > 0 ? `, event ${String(source.part + 1)} of it` : ''}
    {/if}
    of the read from <code>{origin}</code>
  </dd>
  {@render fact('Source type', source.type, 'None (not a JSON event of the runner)')}
  {@render time('Log time', source.logTime)}
  {@render time('Runner time', source.runnerTime)}
  {#if event.kind === 'tool_result'}
    {@render fact('Status', STATUS_LABELS[event.toolResult.status], '')}
    {@render fact(
      'Exit code',
      event.toolResult.exitCode === undefined ? undefined : String(event.toolResult.exitCode),
      'Not reported',
    )}
  {:else if event.kind === 'edit'}
    {@render fact('Change given', diff, 'No diff and no text: only the path changed is known')}
  {/if}
  {@render fact(
    'Cut by Lucarne',
    event.truncated === undefined ? undefined : event.truncated.join(', '),
    'Nothing',
  )}
</dl>

<style>
  .facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.125rem 0.75rem;
    margin: 0;
    font-size: 0.8125rem;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .copies {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 0.125rem;
  }
  .unknown {
    color: var(--text-muted);
    font-style: italic;
  }
</style>
