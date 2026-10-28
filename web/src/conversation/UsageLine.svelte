<script lang="ts">
  import type { EventOf } from '../lib/conversation';
  import { USAGE_SCOPE_LABELS } from '../lib/labels';
  import { usageParts } from '../lib/usage';

  let { event }: { event: EventOf<'usage'> } = $props();

  const u = $derived(event.usage);
  // Only the counters the runner reported, as it named them.
  const parts = $derived(usageParts(u));
</script>

<p class="usage">
  <span class="scope">Usage, {USAGE_SCOPE_LABELS[u.scope]}</span>
  {#each parts as part, i (i)}<span>{part}</span>{/each}
</p>

<style>
  .usage {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.75rem;
    color: var(--text-muted);
    font-size: 0.8125rem;
  }
  .scope {
    font-weight: 600;
  }
</style>
