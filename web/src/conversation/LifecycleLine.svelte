<script lang="ts">
  import type { LifecyclePhase, Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { LIFECYCLE_LABELS } from '../lib/labels';
  import { safeHref } from '../lib/links';
  import OutputBlock from './OutputBlock.svelte';
  import Pulse from './Pulse.svelte';
  import Truncated from './Truncated.svelte';

  let {
    event,
    limits,
    ongoing = false,
  }: { event: EventOf<'lifecycle'>; limits: Limits; ongoing?: boolean } = $props();

  /** Output values longer than this are shown as a block. */
  const INLINE_VALUE_CHARS = 120;

  /** The phases that say work began but has not finished: while the attempt is still being read. */
  const IN_PROGRESS = new Set<LifecyclePhase>(['setup_started', 'step_started', 'turn_started']);

  const life = $derived(event.lifecycle);
  const working = $derived(ongoing && IN_PROGRESS.has(life.phase));
  const failed = $derived(
    life.phase === 'setup_failed' ||
      life.phase === 'turn_failed' ||
      life.phase === 'limit_hit' ||
      life.isError === true,
  );
  const facts = $derived(
    [
      life.model === undefined ? '' : `model ${life.model}`,
      life.runnerVersion === undefined ? '' : `version ${life.runnerVersion}`,
      life.subtype === undefined ? '' : life.subtype,
      life.stopReason === undefined ? '' : `stop reason ${life.stopReason}`,
      life.isError === true ? 'reported as an error' : '',
      life.exitCode === undefined ? '' : `exit code ${String(life.exitCode)}`,
      life.sessionId === undefined ? '' : `session ${life.sessionId}`,
    ].filter((fact) => fact !== ''),
  );
</script>

<div class="lifecycle" class:failed>
  <p role={working ? 'status' : undefined}>
    <span class="phase">{LIFECYCLE_LABELS[life.phase]}</span>
    {#if working}<Pulse />{/if}
    {#each facts as fact, i (i)}<span class="fact">{fact}</span>{/each}
  </p>
  {#if life.detail !== undefined}<OutputBlock label="Detail" text={life.detail} />{/if}
  {#if life.outputs}
    <dl class="outputs">
      {#each life.outputs as output, i (i)}
        {@const href = safeHref(output.value)}
        <dt>{output.key}</dt>
        <dd>
          {#if href !== null}
            <a {href} target="_blank" rel="noopener noreferrer nofollow">{output.value}</a>
          {:else if output.value.length > INLINE_VALUE_CHARS}
            <OutputBlock label="Value" text={output.value} />
          {:else}
            <code>{output.value}</code>
          {/if}
        </dd>
      {/each}
    </dl>
  {/if}
  <Truncated fields={event.truncated} {limits} />
</div>

<style>
  .lifecycle {
    color: var(--text-muted);
    font-size: 0.875rem;
    border-top: 1px dashed var(--border);
    padding-top: 0.25rem;
  }
  p {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.75rem;
  }
  .phase {
    font-weight: 600;
  }
  .failed .phase {
    color: var(--error);
  }
  .fact {
    overflow-wrap: anywhere;
  }
  .outputs {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.125rem 1rem;
    margin: 0.25rem 0;
    color: var(--text);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
