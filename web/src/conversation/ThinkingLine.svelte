<script lang="ts">
  import { LIFECYCLE_LABELS } from '../lib/labels';
  import Pulse from './Pulse.svelte';

  // A batch of the runner's thinking progress: the model at work, animated while it still is.
  let { ongoing }: { ongoing: boolean } = $props();
</script>

<p class="thinking" class:ongoing role={ongoing ? 'status' : undefined}>
  <span class="label">{LIFECYCLE_LABELS.thinking}</span>
  {#if ongoing}
    <Pulse />
  {/if}
</p>

<style>
  .thinking {
    margin: 0;
    color: var(--text-muted);
    font-size: 0.875rem;
    display: flex;
    align-items: center;
    gap: 0.375rem;
  }
  .ongoing .label {
    font-style: italic;
  }
</style>
