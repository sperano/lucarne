<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import MediaList from './MediaList.svelte';
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
<MediaList {media} />
<Truncated fields={event.truncated} {limits} />
