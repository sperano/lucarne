<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { parseMarkdown } from '../lib/markdown';
  import Markdown from './Markdown.svelte';
  import Truncated from './Truncated.svelte';

  let {
    event,
    limits,
    open = false,
  }: { event: EventOf<'message'>; limits: Limits; open?: boolean } = $props();

  const ROLE_LABELS = { user: 'User', assistant: 'Assistant' } as const;

  const message = $derived(event.message);
  const blocks = $derived(parseMarkdown(message.text));
</script>

<article class="message {message.role}" aria-label="{ROLE_LABELS[message.role]} message">
  {#if message.type === 'reasoning'}
    <details class="reasoning" {open}>
      <summary><span class="role">{ROLE_LABELS[message.role]}</span> reasoning</summary>
      <Markdown {blocks} />
    </details>
  {:else}
    <p class="role">{ROLE_LABELS[message.role]}</p>
    {#if message.text === ''}
      <p class="note">The message is empty.</p>
    {:else}
      <Markdown {blocks} />
    {/if}
  {/if}
  <Truncated fields={event.truncated} {limits} />
</article>

<style>
  .message {
    padding: 0.25rem 0.75rem;
    border-radius: var(--radius);
  }
  .message.user {
    background: var(--user-bg);
  }
  .role {
    margin: 0;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--text-muted);
  }
  .reasoning {
    color: var(--text-muted);
  }
  .reasoning summary {
    cursor: pointer;
    font-size: 0.875rem;
  }
  .reasoning .role {
    display: inline;
  }
</style>
