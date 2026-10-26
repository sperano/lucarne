<script lang="ts">
  import type { Block } from '../lib/markdown';
  import InlineView from './InlineView.svelte';
  import Markdown from './Markdown.svelte';

  let { blocks }: { blocks: readonly Block[] } = $props();

  // Message headings sit below the page's and the run's (h1, h2).
  const HEADING_OFFSET = 2;
  const MAX_HEADING = 6;

  function headingTag(level: number): string {
    return `h${String(Math.min(MAX_HEADING, level + HEADING_OFFSET))}`;
  }
</script>

{#each blocks as block, i (i)}
  {#if block.type === 'paragraph'}
    <p><InlineView nodes={block.children} /></p>
  {:else if block.type === 'heading'}
    <svelte:element this={headingTag(block.level)} class="md-heading"
      ><InlineView nodes={block.children} /></svelte:element
    >
  {:else if block.type === 'code'}
    <figure class="md-code">
      {#if block.info !== ''}<figcaption class="note">{block.info}</figcaption>{/if}
      <pre><code>{block.text}</code></pre>
    </figure>
  {:else if block.type === 'quote'}
    <blockquote><Markdown blocks={block.children} /></blockquote>
  {:else if block.type === 'list' && block.ordered}
    <ol start={block.start}>
      {#each block.items as item, j (j)}<li><Markdown blocks={item} /></li>{/each}
    </ol>
  {:else if block.type === 'list'}
    <ul>
      {#each block.items as item, j (j)}<li><Markdown blocks={item} /></li>{/each}
    </ul>
  {:else if block.type === 'table'}
    <div class="md-table">
      <table>
        <thead>
          <tr>
            {#each block.header as cell, j (j)}<th><InlineView nodes={cell} /></th>{/each}
          </tr>
        </thead>
        <tbody>
          {#each block.rows as row, j (j)}
            <tr
              >{#each row as cell, k (k)}<td><InlineView nodes={cell} /></td>{/each}</tr
            >
          {/each}
        </tbody>
      </table>
    </div>
  {:else}
    <hr />
  {/if}
{/each}

<style>
  p {
    margin: 0.5rem 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .md-heading {
    margin: 0.75rem 0 0.25rem;
    font-size: 1rem;
  }
  .md-code {
    margin: 0.5rem 0;
  }
  .md-code figcaption {
    margin: 0 0 0.125rem;
  }
  blockquote {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }
  ol,
  ul {
    margin: 0.25rem 0;
    padding-left: 1.5rem;
  }
  li > :global(p) {
    margin: 0.125rem 0;
  }
  .md-table {
    overflow-x: auto;
  }
  table {
    border-collapse: collapse;
    margin: 0.5rem 0;
  }
  th,
  td {
    border: 1px solid var(--border);
    padding: 0.25rem 0.5rem;
    text-align: left;
    vertical-align: top;
  }
  th {
    background: var(--surface);
  }
  :global(code) {
    overflow-wrap: anywhere;
  }
  p :global(code),
  li :global(code),
  td :global(code) {
    background: var(--code-bg);
    border-radius: 4px;
    padding: 0 0.25rem;
  }
</style>
