<script lang="ts">
  import ItemView from '../conversation/ItemView.svelte';
  import type { CoverageState, Limits, TranscriptEvent } from '../lib/contract';
  import type { Conversation } from '../lib/conversation';
  import { formatTime } from '../lib/format';
  import { EVENT_KIND_LABELS } from '../lib/labels';

  let {
    conversation,
    event,
    coverage,
    limits,
  }: {
    conversation: Conversation;
    /** The event selected, in conversation. */
    event: TranscriptEvent;
    coverage: CoverageState;
    limits: Limits;
  } = $props();

  // The item that shows the event: a call with its result and edits, or the event alone.
  const item = $derived(conversation.items[conversation.itemOf.get(event.id) ?? -1]);
  const grouped = $derived(item !== undefined && item.kind === 'tool' && item.id !== event.id);
  const source = $derived(event.source);
</script>

{#snippet time(label: string, value: string | undefined)}
  <dt>{label}</dt>
  {#if value === undefined}
    <dd class="unknown">Unknown</dd>
  {:else}
    <dd><time datetime={value}>{formatTime(value)}</time></dd>
  {/if}
{/snippet}

<article class="detail" aria-label="{EVENT_KIND_LABELS[event.kind]} detail">
  <h3>{EVENT_KIND_LABELS[event.kind]}</h3>
  <dl class="facts">
    <dt>Event</dt>
    <dd><code>{event.id}</code></dd>
    <dt>Log line</dt>
    <dd>{source.line}{source.part > 0 ? `, part ${String(source.part)}` : ''}</dd>
    <dt>Source type</dt>
    {#if source.type === undefined}
      <dd class="unknown">None (not a JSON event of the runner)</dd>
    {:else}
      <dd><code>{source.type}</code></dd>
    {/if}
    {@render time('Log time', source.logTime)}
    {@render time('Runner time', source.runnerTime)}
  </dl>
  {#if grouped && item?.kind === 'tool'}
    <p class="note">Shown with the call it belongs to, {item.call.toolCall.name}.</p>
  {/if}
  {#if item}
    <div class="body">
      <ItemView {item} {conversation} {coverage} {limits} targeted={true} />
    </div>
  {/if}
</article>

<style>
  .detail {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  h3 {
    margin: 0;
    font-size: 1rem;
  }
  .facts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.125rem 0.75rem;
    margin: 0;
    font-size: 0.8125rem;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .unknown {
    color: var(--text-muted);
    font-style: italic;
  }
  p {
    margin: 0;
  }
</style>
