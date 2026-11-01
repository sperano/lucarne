<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { formatBytes, utf8Length } from '../lib/format';
  import { RAW_REASON_LABELS } from '../lib/labels';
  import { isDamaged } from '../lib/notices';
  import Truncated from './Truncated.svelte';

  let {
    event,
    limits,
    open = false,
  }: { event: EventOf<'raw'>; limits: Limits; open?: boolean } = $props();

  const raw = $derived(event.raw);
  // text holds the start of a longer line when the source line passed maxRawBytes.
  const held = $derived(utf8Length(raw.text));
  // A damaged line is part of the transcript that cannot be shown; any other is a runner notice.
  const damaged = $derived(isDamaged(event));
</script>

{#if raw.reason === 'text'}
  <figure class="raw-text">
    <figcaption class="visually-hidden">{RAW_REASON_LABELS.text}</figcaption>
    <pre>{raw.text}</pre>
  </figure>
{:else}
  <details class="raw" class:damaged {open}>
    <summary>
      {#if damaged}
        <span class="mark" aria-hidden="true">⚠</span>
        <span class="label">Damaged line</span>
        <span>{RAW_REASON_LABELS[raw.reason]}</span>
      {:else}
        <span class="name">{event.source.type ?? 'line'}</span>
        <span>shown as written</span>
      {/if}
      <span>{formatBytes(raw.bytes)}, line {event.source.line}</span>
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
    font-size: 0.8125rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .raw summary > span + span {
    margin-left: 0.75rem;
  }
  .name {
    font-family: var(--font-code);
  }
  .damaged .mark,
  .damaged .label {
    color: var(--error);
    font-weight: 600;
  }
</style>
