<script lang="ts">
  import type { Limits } from '../lib/contract';
  import { formatBytes } from '../lib/format';
  import { fieldLimit } from '../lib/truncation';

  let { fields, limits }: { fields: readonly string[] | undefined; limits: Limits } = $props();

  const cut = $derived(
    (fields ?? []).map((field) => `${field} (${formatBytes(fieldLimit(field, limits))})`),
  );
</script>

{#if cut.length > 0}
  <p class="note truncated">Cut to the size limit by Lucarne: {cut.join(', ')}.</p>
{/if}
