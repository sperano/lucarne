<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { whenVisible } from '../conversation/visible';
  import type { CoverageState, TranscriptEvent } from '../lib/contract';
  import type { Conversation } from '../lib/conversation';
  import { formatCount } from '../lib/format';
  import { keepEnd, moveSelection, reveal, rowOf } from '../lib/inspector';
  import {
    earlier,
    firstWindow,
    lastWindow,
    later,
    reanchor,
    type ItemWindow,
  } from '../lib/window';

  let {
    conversation,
    events,
    matches = new Set(),
    coverage,
    selected,
    focusOnMount = true,
    onselect,
    onopen = () => {},
  }: {
    conversation: Conversation;
    /** The events listed, in order: the attempt's, or those the filters let through. */
    events: readonly TranscriptEvent[];
    /** The IDs of the events that match the search. */
    matches?: ReadonlySet<string>;
    coverage: CoverageState;
    /** The ID of the event selected; it may not be in the list. */
    selected: string | undefined;
    /** Whether a linked event's row takes the focus once shown, as after a view switch. */
    focusOnMount?: boolean;
    onselect: (eventId: string) => void;
    /** Enter on the list: the reader goes to the selected event's detail. */
    onopen?: () => void;
  } = $props();

  /** The prefix of a row's element ID, which aria-activedescendant names. */
  const ROW_ID_PREFIX = 'inspect-';

  const total = $derived(events.length);
  const positionOf = $derived(new Map(events.map((e, i) => [e.id, i])));
  const selectedIndex = $derived(selected === undefined ? undefined : positionOf.get(selected));

  // A linked event opens around it; else a running attempt at its end, a finished one at its start.
  let range: ItemWindow = $state(
    untrack(() => {
      if (selectedIndex !== undefined) return reveal({ start: 0, end: 0 }, selectedIndex, total);
      return coverage === 'following' || coverage === 'pending'
        ? lastWindow(total)
        : firstWindow(total);
    }),
  );
  const shown = $derived(events.slice(range.start, range.end));
  let list: HTMLElement | undefined = $state();
  // The IDs rendered and the total they were part of, to keep them in place when the list changes.
  let shownIds: readonly string[] | undefined;
  let shownTotal = untrack(() => total);

  $effect.pre(() => {
    const now = positionOf;
    untrack(() => {
      if (shownIds !== undefined) {
        const wasAtEnd = range.end === shownTotal;
        range = keepEnd(reanchor(shownIds, now, now.size), wasAtEnd, now.size);
      }
      shownTotal = now.size;
    });
  });

  $effect.pre(() => {
    shownIds = shown.map((e) => e.id);
  });

  // A linked event: bring its row into view once rendered, the keys ready to move from it.
  // The event selected last, by the list or by its first render: one selected elsewhere is revealed.
  let revealed = untrack(() => selected);
  $effect(() => {
    const linked = untrack(() => selected);
    if (linked === undefined || untrack(() => selectedIndex) === undefined) return;
    if (untrack(() => focusOnMount)) list?.focus({ preventScroll: true });
    void showRow(linked, 'center');
  });

  // An event selected elsewhere (a search result, a related event): its row is brought into view.
  $effect(() => {
    const id = selected;
    const index = selectedIndex;
    if (id === revealed) return;
    revealed = id;
    if (index === undefined) return;
    range = reveal(
      untrack(() => range),
      index,
      total,
    );
    void showRow(id, 'center');
  });

  const active = $derived(
    selectedIndex !== undefined && selectedIndex >= range.start && selectedIndex < range.end
      ? rowId(events[selectedIndex]?.id ?? '')
      : undefined,
  );

  function rowId(eventId: string): string {
    return `${ROW_ID_PREFIX}${eventId}`;
  }

  function select(index: number, block: ScrollLogicalPosition): void {
    const event = events[index];
    if (event === undefined) return;
    range = reveal(range, index, total);
    revealed = event.id;
    onselect(event.id);
    void showRow(event.id, block);
  }

  async function showRow(eventId: string | undefined, block: ScrollLogicalPosition): Promise<void> {
    if (eventId === undefined) return;
    await tick();
    document.getElementById(rowId(eventId))?.scrollIntoView({ block });
  }

  function onkeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter' && selectedIndex !== undefined) {
      e.preventDefault();
      onopen();
      return;
    }
    const next = moveSelection(e.key, selectedIndex, total);
    if (next === undefined) return;
    e.preventDefault();
    select(next, 'nearest');
  }

  // The listbox takes the clicks of its options, as it takes their keys.
  function onclick(e: MouseEvent): void {
    list?.focus({ preventScroll: true });
    const option = e.target instanceof Element ? e.target.closest('[role=option]') : null;
    const index = positionOf.get(option?.getAttribute('data-event') ?? '');
    if (index !== undefined) select(index, 'nearest');
  }

  function timeOf(time: string | undefined): string {
    // RFC 3339: the time of day follows the T; the date is in the detail.
    return time === undefined ? '' : time.slice(time.indexOf('T') + 1, time.indexOf('T') + 9);
  }
</script>

<div class="event-list">
  <p class="position muted" aria-live="polite">
    {#if total === 0}
      No events yet.
    {:else}
      Events {formatCount(range.start + 1)}–{formatCount(range.end)} of {formatCount(total)}
    {/if}
  </p>
  {#if range.start > 0}
    <button type="button" class="more" onclick={() => (range = earlier(range))}>
      Show earlier events ({formatCount(range.start)} before these)
    </button>
  {/if}
  <ol
    class="rows"
    role="listbox"
    tabindex="0"
    aria-label="Events of this attempt"
    aria-activedescendant={active}
    bind:this={list}
    {onkeydown}
    {onclick}
  >
    {#each shown as event (event.id)}
      {@const row = rowOf(event, conversation, coverage)}
      {@const time = event.source.logTime ?? event.source.runnerTime}
      <li
        id={rowId(event.id)}
        role="option"
        aria-selected={event.id === selected}
        class:failed={row.failed}
        class:match={matches.has(event.id)}
        data-event={event.id}
      >
        <span class="line" title="Log line"
          >{event.source.line}{event.source.part > 0 ? `.${String(event.source.part)}` : ''}</span
        >
        <time class="time" datetime={time}>{timeOf(time)}</time>
        <span class="kind">{row.kind}</span>
        <span class="summary">{row.summary}</span>
        {#if row.status}<span class="status {row.status.tone}">{row.status.label}</span>{/if}
        {#if matches.has(event.id)}<span class="visually-hidden">, matches the search</span>{/if}
      </li>
    {/each}
  </ol>
  {#if range.end < total}
    {#key range.end}
      <div class="sentinel" {@attach whenVisible(() => (range = later(range, total)))}></div>
    {/key}
    <button type="button" class="more" onclick={() => (range = later(range, total))}>
      Show later events ({formatCount(total - range.end)} after these)
    </button>
  {/if}
</div>

<style>
  .position {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
  }
  .rows {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: 0.8125rem;
  }
  .rows:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  li {
    display: grid;
    grid-template-columns: 3.5rem 4.5rem 6rem minmax(0, 1fr) max-content;
    gap: 0.5rem;
    align-items: baseline;
    padding: 0.125rem 0.5rem;
    border-bottom: 1px solid var(--border);
    cursor: pointer;
    content-visibility: auto;
    contain-intrinsic-size: auto 1.5rem;
  }
  li:last-child {
    border-bottom: 0;
  }
  li[aria-selected='true'] {
    background: var(--info-bg);
    color: var(--info-text);
    box-shadow: inset 3px 0 0 var(--focus);
  }
  .failed {
    box-shadow: inset 3px 0 0 var(--error);
  }
  .match {
    background: var(--warn-bg);
    color: var(--warn-text);
  }
  .match .line,
  .match .time,
  .match .status {
    color: inherit;
  }
  .line,
  .time {
    font-family: var(--font-code);
    color: var(--text-muted);
    text-align: right;
  }
  li[aria-selected='true'] .line,
  li[aria-selected='true'] .time {
    color: inherit;
  }
  .kind {
    font-weight: 600;
  }
  .summary {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
  li[aria-selected='true'] .status {
    color: inherit;
  }
  .more {
    display: block;
    margin: 0.5rem auto;
  }
  .sentinel {
    height: 1px;
  }
</style>
