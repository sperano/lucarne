<script lang="ts">
  import type { Snapshot } from '../lib/contract';
  import { buildConversation } from '../lib/conversation';
  import { attemptOfEvent, selectAttempt } from '../lib/selection';
  import RunHeader from './RunHeader.svelte';
  import Transcript from './Transcript.svelte';

  let {
    snapshot,
    attempt,
    event,
    onselectattempt,
  }: {
    snapshot: Snapshot;
    /** The attempt the link names, if any. */
    attempt: string | undefined;
    /** The event of the link's #event= fragment, if any. */
    event: string | undefined;
    onselectattempt: (attemptId: string) => void;
  } = $props();

  const selection = $derived(selectAttempt(snapshot.attempts, attempt, event));
  const selected = $derived(selection.attempt);
  const conversation = $derived(
    selected ? buildConversation(snapshot.events, selected.id) : undefined,
  );
  // An event link is honoured only in the attempt it belongs to.
  const eventHere = $derived(
    event !== undefined && selected !== undefined && attemptOfEvent(event) === selected.id,
  );
  const target = $derived(
    eventHere && event !== undefined && conversation?.itemOf.has(event) ? event : undefined,
  );
</script>

<div class="conversation">
  <RunHeader
    run={snapshot.run}
    attempts={snapshot.attempts}
    {selected}
    stream={snapshot.stream}
    onselect={onselectattempt}
  />
  {#if selection.missing !== undefined}
    <p class="notice error" role="alert">
      This run has no attempt <code>{selection.missing}</code>: the link names a pod this run does
      not have, or no longer lists. Choose one of its attempts above.
    </p>
  {:else if !selected}
    <p class="notice info" role="note">
      This run has no attempt yet: its pod has not been created.
    </p>
  {/if}
  {#if event !== undefined && selected !== undefined && target === undefined}
    <p class="notice warning" role="alert">
      {#if eventHere}
        The linked event <code>{event}</code> is not in this transcript: the log was read again since
        the link was made, or the run dropped the event to stay within its bounds.
      {:else}
        The linked event <code>{event}</code> belongs to another attempt than the one shown.
      {/if}
    </p>
  {/if}
  {#if conversation && selected}
    <Transcript
      {conversation}
      coverage={selected.coverage.state}
      limits={snapshot.limits}
      {target}
    />
  {/if}
</div>

<style>
  .conversation {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
</style>
