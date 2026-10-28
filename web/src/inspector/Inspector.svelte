<script lang="ts">
  import { untrack } from 'svelte';
  import SelectionNotices from '../conversation/SelectionNotices.svelte';
  import StreamStatus from '../conversation/StreamStatus.svelte';
  import {
    filterEvents,
    filtering,
    kindCounts,
    NO_FILTER,
    searchEvents,
    stepMatch,
    toolNames,
    type EventFilter,
  } from '../lib/filters';
  import type { Connection } from '../lib/live';
  import type { RunView } from '../lib/runstore';
  import { attemptOfEvent, selectAttempt, type Selection } from '../lib/selection';
  import { SourceCache, type LoadSource } from '../lib/sources';
  import EventDetail from './EventDetail.svelte';
  import EventFilters from './EventFilters.svelte';
  import EventList from './EventList.svelte';
  import RunContext from './RunContext.svelte';

  let {
    view,
    connection,
    attempt,
    event,
    onselectattempt,
    onselectevent,
    onsignin = () => {},
    loadSource,
  }: {
    view: RunView;
    /** How the live stream stands; absent for a run shown as loaded, with no stream. */
    connection?: Connection | undefined;
    /** The attempt the link names, if any. */
    attempt: string | undefined;
    /** The event selected: the link's #event= fragment, then the reader's choice. */
    event: string | undefined;
    onselectattempt: (attemptId: string) => void;
    onselectevent: (eventId: string) => void;
    onsignin?: () => void;
    /** Loads the source line of one of the run's events; absent where there is no server to ask. */
    loadSource?: LoadSource | undefined;
  } = $props();

  // One cache for the page: the run, and so the lines, do not change under it.
  const sources = untrack(() => (loadSource ? new SourceCache(loadSource) : undefined));
  let query = $state('');
  let filter: EventFilter = $state.raw(NO_FILTER);
  // Once the reader changed a filter, the list it rebuilds leaves the focus where the reader is.
  let refiltered = $state(false);

  const live = $derived(
    connection !== undefined && connection.state !== 'ended' && connection.state !== 'stopped',
  );
  const finished = $derived(view.run.phase === 'succeeded' || view.run.phase === 'failed');
  // Without a link naming one, the attempt shown first stays: a newer one is offered, not imposed.
  let held: string | undefined = $state();
  const latest = $derived(view.attempts.at(-1)?.id);
  $effect.pre(() => {
    held ??= latest;
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
  const eventHere = $derived(
    event !== undefined && selected !== undefined && attemptOfEvent(event) === selected.id,
  );
  const shownEvent = $derived(
    eventHere && event !== undefined && conversation?.itemOf.has(event)
      ? conversation.events.find((e) => e.id === event)
      : undefined,
  );
  const listed = $derived(conversation ? filterEvents(conversation, filter) : []);
  const matches = $derived(searchEvents(listed, query));
  const matchOrder = $derived(listed.filter((e) => matches.has(e.id)).map((e) => e.id));
  const listedIndex = $derived.by(() => {
    if (shownEvent === undefined) return undefined;
    const i = listed.indexOf(shownEvent);
    return i === -1 ? undefined : i;
  });
  const current = $derived.by(() => {
    if (shownEvent === undefined) return undefined;
    const i = matchOrder.indexOf(shownEvent.id);
    return i === -1 ? undefined : i;
  });
  const hidden = $derived(shownEvent !== undefined && listedIndex === undefined);
  let detail: HTMLElement | undefined = $state();
  let list: HTMLElement | undefined = $state();

  function changeFilter(next: EventFilter): void {
    filter = next;
    refiltered = true;
  }

  function resetFilters(): void {
    query = '';
    changeFilter(NO_FILTER);
  }

  function step(direction: 1 | -1): void {
    const i = stepMatch(listed, matches, listedIndex, direction);
    const next = i === undefined ? undefined : listed[i];
    if (next) onselectevent(next.id);
  }

  function showNewer(): void {
    if (latest !== undefined) onselectattempt(latest);
  }

  // Escape in the detail goes back to the list, where the keys move the selection.
  function onkeydown(e: KeyboardEvent): void {
    if (e.key !== 'Escape' || !(e.target instanceof Node) || !detail?.contains(e.target)) return;
    e.preventDefault();
    list?.querySelector<HTMLElement>('[role=listbox]')?.focus();
  }
</script>

<svelte:window {onkeydown} />

<div class="inspector">
  <aside class="pane context" aria-label="Run context">
    <RunContext
      run={view.run}
      attempts={view.attempts}
      {selected}
      stream={view.stream}
      events={conversation?.events ?? []}
      onselect={onselectattempt}
    />
  </aside>
  <section class="events" aria-label="Events" bind:this={list}>
    {#if connection}
      <StreamStatus {connection} run={view.run} {onsignin} />
    {/if}
    {#if newer}
      <p class="notice info" role="note">
        A newer attempt of this run started. <button type="button" onclick={showNewer}
          >Show it</button
        >
      </p>
    {/if}
    <SelectionNotices
      {selection}
      {finished}
      {live}
      {event}
      {eventHere}
      found={shownEvent !== undefined}
    />
    {#if conversation && selected}
      <EventFilters
        bind:query
        {filter}
        kinds={kindCounts(conversation.events)}
        tools={toolNames(conversation)}
        listed={listed.length}
        total={conversation.events.length}
        matches={matches.size}
        {current}
        onfilter={changeFilter}
        onstep={step}
        onreset={resetFilters}
      />
      {#if hidden}
        <p class="notice info" role="note">
          The selected event is not among the events the filters list.
          <button
            type="button"
            onclick={() => {
              changeFilter(NO_FILTER);
            }}>List every event</button
          >
        </p>
      {/if}
      {#key `${selected.id} ${JSON.stringify(filter)}`}
        <EventList
          {conversation}
          events={listed}
          {matches}
          coverage={selected.coverage.state}
          selected={shownEvent?.id}
          focusOnMount={!refiltered}
          onselect={onselectevent}
          onopen={() => detail?.focus()}
        />
      {/key}
      {#if filtering(filter) && listed.length === 0}
        <p class="muted">No event of this attempt passes the filters.</p>
      {/if}
    {/if}
  </section>
  <section class="pane detail" aria-label="Event detail" tabindex="-1" bind:this={detail}>
    {#if conversation && selected && shownEvent}
      <EventDetail
        {conversation}
        event={shownEvent}
        attempt={selected}
        stream={view.stream}
        limits={view.limits}
        runKey={view.run.key}
        {sources}
        onselect={onselectevent}
        {onsignin}
      />
    {:else if conversation && selected}
      <p class="muted">
        Select an event in the list: ↑ and ↓ move, Page Up and Page Down by a page, Home and End to
        the ends, Enter comes here and Escape goes back.
      </p>
    {/if}
  </section>
</div>

<style>
  .inspector {
    display: grid;
    grid-template-columns: 18rem minmax(0, 1fr) minmax(0, 1fr);
    gap: 1rem;
    align-items: start;
  }
  .events {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-width: 0;
  }
  .pane {
    position: sticky;
    top: 0;
    max-height: 100vh;
    overflow: auto;
    padding: 0.25rem 0.25rem 1rem;
    box-sizing: border-box;
  }
  .detail:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
</style>
