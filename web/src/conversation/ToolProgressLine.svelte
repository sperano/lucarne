<script lang="ts">
  import type { EventOf } from '../lib/conversation';
  import { formatDuration } from '../lib/format';
  import { TOOL_PROGRESS_LABELS } from '../lib/labels';
  import { isActiveProgressPhase, isFailedProgressPhase } from '../lib/tool-progress';
  import Pulse from './Pulse.svelte';

  let { event, ongoing = false }: { event: EventOf<'tool_progress'>; ongoing?: boolean } = $props();

  const progress = $derived(event.toolProgress);
  const working = $derived(ongoing && isActiveProgressPhase(progress.phase));
  const failed = $derived(isFailedProgressPhase(progress.phase));
</script>

<p class="tool-progress" class:failed role={working ? 'status' : undefined}>
  <span class="phase">Tool {TOOL_PROGRESS_LABELS[progress.phase]}</span>
  {#if working}<Pulse />{/if}
  {#if progress.detail !== undefined}<span>{progress.detail}</span>{/if}
  {#if progress.subtype !== undefined}<span class="muted">{progress.subtype}</span>{/if}
  {#if progress.background === true}<span class="muted">background</span>{/if}
  {#if progress.elapsedMs !== undefined}
    <span class="muted">{formatDuration(progress.elapsedMs)}</span>
  {/if}
</p>

<style>
  .tool-progress {
    margin: 0;
    color: var(--text-muted);
    font-size: 0.875rem;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.375rem 0.75rem;
  }
  .phase {
    font-weight: 600;
  }
  .failed .phase {
    color: var(--error);
  }
</style>
