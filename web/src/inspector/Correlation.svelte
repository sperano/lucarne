<script lang="ts">
  import type { Attempt, Stream, TranscriptEvent } from '../lib/contract';
  import type { Conversation } from '../lib/conversation';
  import { callEvidence, correlate, RELATION_LABELS } from '../lib/evidence';
  import { formatCount } from '../lib/format';
  import { rowOf } from '../lib/inspector';

  let {
    event,
    conversation,
    attempt,
    stream,
    onselect,
  }: {
    event: TranscriptEvent;
    conversation: Conversation;
    attempt: Attempt;
    stream: Stream;
    /** Selects a related event, as the list does. */
    onselect: (eventId: string) => void;
  } = $props();

  const correlation = $derived(correlate(event, conversation));
  const evidence = $derived(callEvidence(correlation, attempt, stream));
</script>

{#if correlation.callId !== undefined}
  <section class="correlation" aria-label="Related events">
    <h4>Call <code>{correlation.callId}</code></h4>
    <ol>
      {#each correlation.related as r (r.event.id)}
        {@const row = rowOf(r.event, conversation, attempt.coverage.state)}
        <li>
          <span class="relation">{RELATION_LABELS[r.relation]}</span>
          <span class="line muted">line {formatCount(r.event.source.line)}</span>
          {#if r.event.id === event.id}
            <span class="summary" aria-current="true">{row.summary} (this event)</span>
          {:else}
            <button
              type="button"
              class="summary"
              onclick={() => {
                onselect(r.event.id);
              }}>{row.summary}</button
            >
          {/if}
          {#if row.status}<span class="status {row.status.tone}">{row.status.label}</span>{/if}
        </li>
      {/each}
    </ol>
    {#if correlation.moreSubcalls > 0}
      <p class="note">
        And {formatCount(correlation.moreSubcalls)} more sub-calls, further down the event list.
      </p>
    {/if}
    {#if evidence !== undefined}
      <p class="notice warning" role="note">{evidence}</p>
    {/if}
  </section>
{/if}

<style>
  .correlation {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.8125rem;
  }
  h4 {
    margin: 0;
    font-size: 0.875rem;
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    grid-template-columns: 7rem 5rem minmax(0, 1fr) max-content;
    gap: 0.5rem;
    align-items: baseline;
  }
  .relation {
    font-weight: 600;
  }
  .summary {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
  }
  button.summary {
    font: inherit;
    background: none;
    border: 0;
    padding: 0;
    color: var(--link);
    text-decoration: underline;
    cursor: pointer;
  }
  .status {
    font-weight: 600;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .status.ok {
    color: var(--ok);
  }
  .status.error {
    color: var(--error);
  }
  p {
    margin: 0;
  }
</style>
