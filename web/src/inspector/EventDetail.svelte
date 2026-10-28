<script lang="ts">
  import ItemView from '../conversation/ItemView.svelte';
  import OutputBlock from '../conversation/OutputBlock.svelte';
  import type { Attempt, Limits, RunKey, Stream, TranscriptEvent } from '../lib/contract';
  import type { Conversation } from '../lib/conversation';
  import { EVENT_KIND_LABELS } from '../lib/labels';
  import type { SourceCache } from '../lib/sources';
  import CopyButton from './CopyButton.svelte';
  import Correlation from './Correlation.svelte';
  import EventFacts from './EventFacts.svelte';
  import SourceView from './SourceView.svelte';

  let {
    conversation,
    event,
    attempt,
    stream,
    limits,
    runKey,
    sources,
    onselect,
    onsignin = () => {},
  }: {
    conversation: Conversation;
    /** The event selected, in conversation. */
    event: TranscriptEvent;
    /** The attempt conversation is of. */
    attempt: Attempt;
    stream: Stream;
    limits: Limits;
    runKey: RunKey;
    sources: SourceCache | undefined;
    /** Selects a related event. */
    onselect: (eventId: string) => void;
    onsignin?: () => void;
  } = $props();

  // The item that shows the event: a call with its result and edits, or the event alone.
  const item = $derived(conversation.items[conversation.itemOf.get(event.id) ?? -1]);
  const grouped = $derived(item !== undefined && item.kind === 'tool' && item.id !== event.id);
  const normalized = $derived(JSON.stringify(event, null, 2));
</script>

<article class="detail" aria-label="{EVENT_KIND_LABELS[event.kind]} detail">
  <h3>{EVENT_KIND_LABELS[event.kind]}</h3>
  <EventFacts {event} {attempt} {runKey} />
  {#if grouped && item?.kind === 'tool'}
    <p class="note">Shown with the call it belongs to, {item.call.toolCall.name}.</p>
  {/if}
  {#if item}
    <div class="body">
      <ItemView {item} {conversation} coverage={attempt.coverage.state} {limits} targeted={true} />
    </div>
  {/if}
  <Correlation {event} {conversation} {attempt} {stream} {onselect} />
  {#key event.id}
    <SourceView eventId={event.id} line={event.source.line} {sources} {limits} {onsignin} />
  {/key}
  <details class="normalized">
    <summary>The event as Lucarne normalized it (JSON)</summary>
    <OutputBlock label="Normalized event" text={normalized} />
    <CopyButton label="Copy the normalized event" text={() => normalized} />
  </details>
</article>

<style>
  .detail {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  h3 {
    margin: 0;
    font-size: 1rem;
  }
  p {
    margin: 0;
  }
  summary {
    cursor: pointer;
    font-size: 0.875rem;
  }
</style>
