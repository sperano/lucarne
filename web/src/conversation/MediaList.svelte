<script lang="ts">
  import type { Media } from '../lib/contract';
  import { mediaLabel } from '../lib/media';

  let { media }: { media: readonly Media[] } = $props();
</script>

{#if media.length > 0}
  <figure class="media">
    <figcaption class="note">Images and documents (Lucarne does not show their content)</figcaption>
    <ul aria-label="Images and documents">
      {#each media as entry, i (i)}<li>{mediaLabel(entry)}</li>{/each}
    </ul>
  </figure>
{/if}

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
