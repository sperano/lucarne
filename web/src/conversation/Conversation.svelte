<script lang="ts">
  import { formatCount } from '../lib/format';
  import { shownItems } from '../lib/conversation';
  import type { Connection } from '../lib/live';
  import type { RunView } from '../lib/runstore';
  import { passes, search, type ItemFilter } from '../lib/search';
  import { attemptOfEvent, selectAttempt, type Selection } from '../lib/selection';
  import type { Target } from '../lib/window';
  import Finder from './Finder.svelte';
  import RunHeader from './RunHeader.svelte';
  import SelectionNotices from './SelectionNotices.svelte';
  import StreamStatus from './StreamStatus.svelte';
  import Transcript from './Transcript.svelte';

  let {
    view,
    connection,
    attempt,
    event,
    onselectattempt,
    onsignin = () => {},
  }: {
    view: RunView;
    /** How the live stream stands; absent for a run shown as loaded, with no stream. */
    connection?: Connection | undefined;
    /** The attempt the link names, if any. */
    attempt: string | undefined;
    /** The event of the link's #event= fragment, if any. */
    event: string | undefined;
    onselectattempt: (attemptId: string) => void;
    onsignin?: () => void;
  } = $props();

  // Whether updates may still arrive.
  const live = $derived(
    connection !== undefined && connection.state !== 'ended' && connection.state !== 'stopped',
  );
  // A finished run's pod is not created any more: one it does not list is gone.
  const finished = $derived(view.run.phase === 'succeeded' || view.run.phase === 'failed');
  let following = $state(false);
  // Without a link naming one, the latest attempt is shown; while the reader is paused, the
  // one they read stays, and one the server stops listing is said to be gone.
  let held: string | undefined = $state();
  const latest = $derived(view.attempts.at(-1)?.id);
  $effect.pre(() => {
    const listed = held !== undefined && view.attempts.some((a) => a.id === held);
    if (held === undefined || (following && listed)) held = latest;
  });

  const selection: Selection = $derived.by(() => {
    if (attempt !== undefined || event !== undefined || held === undefined) {
      return selectAttempt(view.attempts, attempt, event);
    }
    const shown = view.attempts.find((a) => a.id === held);
    return shown ? { attempt: shown } : { missing: held };
  });
  const selected = $derived(selection.attempt);
  const newer = $derived(
    attempt === undefined &&
      event === undefined &&
      selected !== undefined &&
      latest !== selected.id,
  );
  const conversation = $derived(selected ? view.conversationOf(selected.id) : undefined);

  // An event link is honoured only in the attempt it belongs to.
  const eventHere = $derived(
    event !== undefined && selected !== undefined && attemptOfEvent(event) === selected.id,
  );
  const linked = $derived(
    eventHere && event !== undefined && conversation?.itemOf.has(event) ? event : undefined,
  );

  let query = $state('');
  let filter: ItemFilter = $state('all');
  let current: string | undefined = $state();
  let request: Target | undefined = $state();
  const items = $derived(
    conversation
      ? shownItems(conversation.items).filter((i) => filter === 'all' || passes(i, filter))
      : [],
  );
  const matches = $derived(search(items, query));
  const matchSet = $derived(new Set(matches));
  const currentIndex = $derived.by(() => {
    const at = current === undefined ? -1 : matches.indexOf(current);
    return at === -1 ? undefined : at;
  });
  // A search result asked for wins over the link until the link changes.
  const target: Target | undefined = $derived(
    request ?? (linked === undefined ? undefined : { event: linked, focus: true }),
  );
  const scope = $derived(scopeOf(items.length, view.stream.droppedEvents, live));

  function step(direction: 1 | -1): void {
    if (matches.length === 0) return;
    const at = currentIndex ?? (direction === 1 ? -1 : 0);
    const next = matches[(at + direction + matches.length) % matches.length];
    if (next === undefined) return;
    current = next;
    request = { event: next, focus: false };
  }

  function scopeOf(count: number, dropped: number, more: boolean): string {
    const what = filter === 'all' ? 'items' : 'items the filter shows';
    const parts = [
      `Searches the ${formatCount(count)} ${what} of this attempt, collapsed content included`,
    ];
    if (dropped > 0)
      parts.push(
        `not the ${formatCount(dropped)} oldest events of the run, dropped to stay within bounds`,
      );
    if (more) parts.push('items that arrive are searched as they come');
    return `${parts.join('; ')}.`;
  }

  function showNewer(): void {
    held = latest;
    following = true;
  }
</script>

<div class="conversation">
  <RunHeader
    run={view.run}
    attempts={view.attempts}
    {selected}
    stream={view.stream}
    onselect={onselectattempt}
  />
  {#if connection}
    <StreamStatus {connection} run={view.run} {onsignin} />
  {/if}
  {#if newer}
    <p class="notice info" role="note">
      A newer attempt of this run started. <button type="button" onclick={showNewer}>Show it</button
      >
    </p>
  {/if}
  <SelectionNotices
    {selection}
    {finished}
    {live}
    {event}
    {eventHere}
    found={linked !== undefined}
  />
  {#if conversation && selected}
    <Finder
      bind:query
      bind:filter
      matches={matches.length}
      current={currentIndex}
      {scope}
      onstep={step}
    />
    {#key selected.id}
      <Transcript
        {conversation}
        coverage={selected.coverage.state}
        limits={view.limits}
        {target}
        {filter}
        matches={matchSet}
        {current}
        {live}
        bind:following
      />
    {/key}
  {/if}
</div>

<style>
  .conversation {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
</style>
