<script lang="ts">
  import type { Attempt } from '../lib/contract';
  import { formatCount } from '../lib/format';

  let { attempt }: { attempt: Attempt } = $props();

  const coverage = $derived(attempt.coverage);
  const losses = $derived(
    [
      coverage.gaps === 0
        ? ''
        : `${formatCount(coverage.gaps)} ${coverage.gaps === 1 ? 'gap' : 'gaps'}`,
      coverage.truncatedLines === 0
        ? ''
        : `${formatCount(coverage.truncatedLines)} ${coverage.truncatedLines === 1 ? 'line' : 'lines'} cut to the line size limit`,
    ].filter((loss) => loss !== ''),
  );
</script>

{#if coverage.state === 'pending'}
  <p class="notice info" role="note">The log of this attempt has not been read yet.</p>
{:else if coverage.state === 'following'}
  <p class="notice info" role="note">
    The attempt is still running: this transcript ends where the log had been read when it was
    loaded.
  </p>
{:else if coverage.state === 'unavailable'}
  <p class="notice warning" role="note">
    The log of this attempt cannot be read now{coverage.reason === undefined
      ? ''
      : `: ${coverage.reason}`}. What was read before is shown.
  </p>
{:else if coverage.state === 'expired'}
  <p class="notice warning" role="note">
    The log of this attempt is gone (pod deleted, Task expired or an older restart){coverage.reason ===
    undefined
      ? ''
      : `: ${coverage.reason}`}. Only what was read before it went is shown.
  </p>
{/if}
{#if losses.length > 0}
  <p class="notice warning" role="note">
    This transcript is incomplete: {losses.join(' and ')}.
  </p>
{:else if coverage.state === 'ended'}
  <p class="note">The log was read to the end with no gap and no cut line.</p>
{/if}
