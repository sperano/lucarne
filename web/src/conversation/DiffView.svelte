<script lang="ts">
  import { diffLines } from '../lib/diff';
  import { formatCount } from '../lib/format';

  let { diff }: { diff: string } = $props();

  /** Diff lines shown until the diff is expanded. */
  const DIFF_PREVIEW_LINES = 40;

  let expanded = $state(false);
  const lines = $derived(diffLines(diff));
  const shown = $derived(expanded ? lines : lines.slice(0, DIFF_PREVIEW_LINES));
</script>

<pre class="diff"><code
    >{#each shown as line, i (i)}<span class="line {line.kind}">{line.text}</span>{/each}</code
  ></pre>
{#if lines.length > DIFF_PREVIEW_LINES}
  <button type="button" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
    {expanded ? 'Show less' : `Show all ${formatCount(lines.length)} lines of the diff`}
  </button>
{/if}

<style>
  .diff {
    padding: 0.25rem 0;
  }
  .line {
    display: block;
    padding: 0 0.75rem;
    min-height: 1lh;
  }
  .add {
    background: var(--diff-add-bg);
    color: var(--diff-add-text);
  }
  .delete {
    background: var(--diff-del-bg);
    color: var(--diff-del-text);
  }
  .hunk {
    background: var(--diff-hunk-bg);
    color: var(--diff-hunk-text);
  }
  .header {
    color: var(--text-muted);
  }
  button {
    margin-top: 0.25rem;
  }
</style>
