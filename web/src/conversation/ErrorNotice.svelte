<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { formatDuration } from '../lib/format';
  import Truncated from './Truncated.svelte';

  let { event, limits }: { event: EventOf<'error'>; limits: Limits } = $props();

  const error = $derived(event.error);
  const retry = $derived.by(() => {
    const r = error.retry;
    if (!r) return undefined;
    const of = r.max === undefined ? '' : ` of ${String(r.max)}`;
    const delay = r.delayMs === undefined ? '' : ` in ${formatDuration(r.delayMs)}`;
    return `Retry ${String(r.attempt)}${of}${delay}.`;
  });
</script>

<div class="notice error" role="note" aria-label="Runner error">
  <p class="head">
    <strong>Error</strong>
    {#if error.code !== undefined}<code>{error.code}</code>{/if}
    {#if error.status !== undefined}<span>HTTP {error.status}</span>{/if}
    {#if retry}<span>{retry}</span>{/if}
  </p>
  <p class="message">{error.message}</p>
  <Truncated fields={event.truncated} {limits} />
</div>

<style>
  p {
    margin: 0;
  }
  .head {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
  }
  .message {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
