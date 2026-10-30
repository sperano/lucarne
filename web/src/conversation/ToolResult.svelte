<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { mediaLabel } from '../lib/media';
  import OutputBlock from './OutputBlock.svelte';
  import Truncated from './Truncated.svelte';

  let { event, limits }: { event: EventOf<'tool_result'>; limits: Limits } = $props();

  const result = $derived(event.toolResult);
  const media = $derived(result.media ?? []);
</script>

{#if result.output === undefined}
  {#if media.length === 0}<p class="note">The result carries no output.</p>{/if}
{:else if result.output === ''}
  <p class="note">The output is empty.</p>
{:else}
  <OutputBlock label={result.status === 'error' ? 'Error output' : 'Output'} text={result.output} />
{/if}
{#if media.length > 0}
  <figure class="media">
    <figcaption class="note">Images and documents (Lucarne does not show their content)</figcaption>
    <ul aria-label="Images and documents">
      {#each media as entry, i (i)}<li>{mediaLabel(entry)}</li>{/each}
    </ul>
  </figure>
{/if}
<Truncated fields={event.truncated} {limits} />

<style>
  .media {
    margin: 0.5rem 0;
  }
  figcaption {
    margin: 0 0 0.125rem;
  }
  ul {
    margin: 0;
    padding-left: 1.25rem;
  }
</style>
