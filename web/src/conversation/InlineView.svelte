<script lang="ts">
  import type { Inline } from '../lib/inline';
  import InlineView from './InlineView.svelte';

  let { nodes }: { nodes: readonly Inline[] } = $props();
</script>

{#each nodes as node, i (i)}{#if node.type === 'text'}{node.text}{:else if node.type === 'code'}<code
      >{node.text}</code
    >{:else if node.type === 'strong'}<strong><InlineView nodes={node.children} /></strong
    >{:else if node.type === 'em'}<em><InlineView nodes={node.children} /></em
    >{:else if node.type === 'del'}<del><InlineView nodes={node.children} /></del>{:else}<a
      href={node.href}
      target="_blank"
      rel="noopener noreferrer nofollow"><InlineView nodes={node.children} /></a
    >{/if}{/each}
