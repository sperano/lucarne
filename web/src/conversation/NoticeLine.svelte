<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { detailLine, noticeName } from '../lib/notices';
  import OutputBlock from './OutputBlock.svelte';
  import Truncated from './Truncated.svelte';

  let {
    event,
    limits,
    open = false,
  }: { event: EventOf<'lifecycle'>; limits: Limits; open?: boolean } = $props();

  const life = $derived(event.lifecycle);
  const name = $derived(noticeName(event.source.type, life.subtype));
  const detail = $derived(detailLine(life.detail));
  const failed = $derived(life.isError === true);
  const facts = $derived(
    [
      life.model === undefined ? '' : `model ${life.model}`,
      life.exitCode === undefined ? '' : `exit code ${String(life.exitCode)}`,
    ].filter((fact) => fact !== ''),
  );
</script>

{#snippet line()}
  {#if failed}<span class="mark" aria-hidden="true">⚠</span>{/if}
  <span class="name">{name}</span>
  {#if detail.first !== ''}<span class="first">{detail.first}</span>{/if}
  {#each facts as fact, i (i)}<span class="fact">{fact}</span>{/each}
{/snippet}

<div class="runner-notice" class:failed>
  {#if life.detail !== undefined && detail.more}
    <details {open}>
      <summary class="line">{@render line()}</summary>
      <OutputBlock label="Detail" text={life.detail} />
    </details>
  {:else}
    <p class="line">{@render line()}</p>
  {/if}
  <Truncated fields={event.truncated} {limits} />
</div>

<style>
  .runner-notice {
    color: var(--text-muted);
    font-size: 0.8125rem;
  }
  .line {
    margin: 0;
    display: flex;
    gap: 0 0.75rem;
    min-width: 0;
    white-space: nowrap;
  }
  summary.line {
    cursor: pointer;
  }
  .name {
    font-family: var(--font-code);
    flex: none;
  }
  .first {
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .fact {
    flex: none;
  }
  .failed .mark,
  .failed .name {
    color: var(--error);
  }
</style>
