<script lang="ts">
  import { tick } from 'svelte';
  import type { CoverageState, Limits } from '../lib/contract';
  import type { Conversation } from '../lib/conversation';
  import { formatCount } from '../lib/format';
  import {
    earlier,
    firstWindow,
    lastWindow,
    later,
    windowAround,
    type ItemWindow,
  } from '../lib/window';
  import ItemView from './ItemView.svelte';
  import { whenVisible } from './visible';

  let {
    conversation,
    coverage,
    limits,
    target,
  }: {
    conversation: Conversation;
    coverage: CoverageState;
    limits: Limits;
    /** The event a #event= link points to; it must be in conversation. */
    target: string | undefined;
  } = $props();

  const total = $derived(conversation.items.length);
  const targetIndex = $derived(target === undefined ? undefined : conversation.itemOf.get(target));
  // Reset whenever the transcript or the target changes; the controls move it.
  let range: ItemWindow = $derived(
    targetIndex === undefined ? firstWindow(total) : windowAround(targetIndex, total),
  );
  const shown = $derived(conversation.items.slice(range.start, range.end));

  $effect(() => {
    if (targetIndex === undefined) return;
    const item = conversation.items[targetIndex];
    if (!item) return;
    void tick().then(() => {
      const element = document.getElementById(anchor(item.id));
      element?.scrollIntoView({ block: 'center' });
      element?.focus({ preventScroll: true });
    });
  });

  function anchor(id: string): string {
    return `event=${id}`;
  }

  function showLater(): void {
    range = later(range, total);
  }

  function showEarlier(): void {
    range = earlier(range);
  }
</script>

<section class="transcript" aria-label="Transcript">
  <nav class="controls" aria-label="Transcript position">
    <span class="muted" aria-live="polite">
      {#if total === 0}
        No events.
      {:else}
        Items {formatCount(range.start + 1)}–{formatCount(range.end)} of {formatCount(total)}
      {/if}
    </span>
    {#if total > 0}
      <button
        type="button"
        disabled={range.start === 0}
        onclick={() => (range = firstWindow(total))}
      >
        Start
      </button>
      <button
        type="button"
        disabled={range.end === total}
        onclick={() => (range = lastWindow(total))}
      >
        End
      </button>
    {/if}
  </nav>
  {#if range.start > 0}
    <button type="button" class="more" onclick={showEarlier}>
      Show earlier items ({formatCount(range.start)} before these)
    </button>
  {/if}
  <ol class="items">
    {#each shown as item, i (item.id)}
      {@const targeted = targetIndex === range.start + i}
      <li id={anchor(item.id)} class:targeted tabindex="-1">
        <ItemView {item} {conversation} {coverage} {limits} {targeted} />
      </li>
    {/each}
  </ol>
  {#if range.end < total}
    {#key range.end}
      <div class="sentinel" {@attach whenVisible(showLater)}></div>
    {/key}
    <button type="button" class="more" onclick={showLater}>
      Show later items ({formatCount(total - range.end)} after these)
    </button>
  {/if}
</section>

<style>
  .controls {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin: 0 0 0.75rem;
  }
  .controls span {
    margin-right: auto;
  }
  .items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .items > li {
    content-visibility: auto;
    contain-intrinsic-size: auto 3rem;
    border-radius: var(--radius);
  }
  .targeted {
    outline: 2px solid var(--focus);
    outline-offset: 4px;
  }
  .more {
    display: block;
    margin: 0.75rem auto;
  }
  .sentinel {
    height: 1px;
  }
</style>
