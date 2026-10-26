<script lang="ts">
  import type { EventOf } from '../lib/conversation';
  import { formatCount, formatDuration } from '../lib/format';
  import { USAGE_SCOPE_LABELS } from '../lib/labels';

  let { event }: { event: EventOf<'usage'> } = $props();

  const u = $derived(event.usage);
  // Only the counters the runner reported, as it named them.
  const parts = $derived(
    [
      count(u.inputTokens, 'input tokens'),
      count(u.outputTokens, 'output tokens'),
      count(u.reasoningTokens, 'reasoning tokens'),
      count(u.cacheReadTokens, 'cache read'),
      count(u.cacheWriteTokens, 'cache write'),
      count(u.totalTokens, 'total tokens'),
      u.costUSD === undefined ? '' : `$${u.costUSD}`,
      u.durationMs === undefined ? '' : formatDuration(u.durationMs),
      u.apiDurationMs === undefined ? '' : `API ${formatDuration(u.apiDurationMs)}`,
      u.turns === undefined ? '' : count(u.turns, u.turns === 1 ? 'turn' : 'turns'),
    ].filter((part) => part !== ''),
  );

  function count(n: number | undefined, unit: string): string {
    return n === undefined ? '' : `${formatCount(n)} ${unit}`;
  }
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
