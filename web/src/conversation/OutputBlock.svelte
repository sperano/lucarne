<script lang="ts">
  import { formatCount } from '../lib/format';
  import { preview } from '../lib/text';

  let {
    text,
    label,
    prefix = '',
  }: {
    text: string;
    /** What the block holds, for its caption and its button. */
    label: string;
    /** Printed before the text and not counted, as `$ ` before a command. */
    prefix?: string;
  } = $props();

  let expanded = $state(false);
  const cut = $derived(preview(text));
  const shown = $derived(expanded ? text : cut.text);
  const showAll = $derived(
    cut.lines > 1 ? `Show all ${formatCount(cut.lines)} lines` : 'Show the whole line',
  );
</script>

<figure class="output">
  <figcaption class="note">{label}</figcaption>
  <pre><code>{prefix}{shown}</code></pre>
  {#if cut.clipped}
    <button type="button" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
      {expanded ? 'Show less' : showAll}
    </button>
  {/if}
</figure>

<style>
  .output {
    margin: 0.5rem 0;
  }
  figcaption {
    margin: 0 0 0.125rem;
  }
  button {
    margin-top: 0.25rem;
  }
</style>
