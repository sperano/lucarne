<script lang="ts">
  import type { Attempt } from '../lib/contract';
  import { COVERAGE_LABELS } from '../lib/labels';

  let {
    attempts,
    selected,
    onselect,
  }: {
    attempts: readonly Attempt[];
    selected: Attempt | undefined;
    onselect: (attemptId: string) => void;
  } = $props();

  function label(attempt: Attempt, index: number): string {
    const { podName, key, coverage } = attempt;
    return `${String(index + 1)}. ${podName}, restart ${String(key.restart)}: ${COVERAGE_LABELS[coverage.state]}`;
  }
</script>

<label class="picker">
  Attempt
  <select
    value={selected?.id ?? ''}
    onchange={(e) => {
      onselect(e.currentTarget.value);
    }}
  >
    {#if !selected}<option value="" disabled>Choose an attempt</option>{/if}
    {#each attempts as attempt, i (attempt.id)}
      <option value={attempt.id}>{label(attempt, i)}</option>
    {/each}
  </select>
</label>

<style>
  .picker {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    font-size: 0.875rem;
  }
  select {
    font: inherit;
    max-width: 100%;
  }
</style>
