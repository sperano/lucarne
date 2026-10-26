<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { STATUS_LABELS } from '../lib/labels';
  import ToolResult from './ToolResult.svelte';

  let {
    event,
    repeated,
    limits,
  }: {
    event: EventOf<'tool_result'>;
    /** Whether the call is in the transcript and already has a result. */
    repeated: boolean;
    limits: Limits;
  } = $props();

  const result = $derived(event.toolResult);
</script>

<article class="orphan" aria-label="Tool result">
  <p class="note">
    {repeated ? 'Another result for call' : 'Result of call'}
    <code>{result.callId}</code>{repeated ? '' : ', which is not in this transcript'}:
    <strong>{STATUS_LABELS[result.status]}</strong>
    {#if result.exitCode !== undefined}, exit {result.exitCode}{/if}
  </p>
  <ToolResult {event} {limits} />
</article>

<style>
  .orphan {
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    padding: 0.25rem 0.75rem;
  }
</style>
