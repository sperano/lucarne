<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { formatBytes, utf8Length } from '../lib/format';
  import { RAW_REASON_LABELS } from '../lib/labels';
  import Truncated from './Truncated.svelte';

  let { event, limits }: { event: EventOf<'raw'>; limits: Limits } = $props();

  const raw = $derived(event.raw);
  // text holds the start of a longer line when the source line passed maxRawBytes.
  const held = $derived(utf8Length(raw.text));
</script>

{#if raw.reason === 'text'}
  <figure class="raw-text">
    <figcaption class="visually-hidden">{RAW_REASON_LABELS.text}</figcaption>
    <pre>{raw.text}</pre>
  </figure>
{:else}
  <details class="raw">
    <summary>
      <span class="label">{RAW_REASON_LABELS[raw.reason]}</span>
      <span class="muted">{formatBytes(raw.bytes)}, line {event.source.line}</span>
    </summary>
    <pre>{raw.text}</pre>
    {#if held < raw.bytes}
      <p class="note">Showing the first {formatBytes(held)} of {formatBytes(raw.bytes)}.</p>
    {/if}
  </details>
{/if}
<Truncated fields={event.truncated} {limits} />

<style>
  .raw-text {
    margin: 0;
  }
  .raw-text pre {
    color: var(--text-muted);
    background: var(--bg);
    border-style: dashed;
  }
  .raw summary {
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.875rem;
  }
  .label {
    font-weight: 600;
  }
</style>
