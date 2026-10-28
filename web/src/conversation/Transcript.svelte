<script lang="ts">
  import { tick, untrack } from 'svelte';
  import type { CoverageState, Limits } from '../lib/contract';
  import { shownItems, type Conversation, type Item } from '../lib/conversation';
  import { formatCount } from '../lib/format';
  import { passes, type ItemFilter } from '../lib/search';
  import {
    earlier,
    firstWindow,
    lastWindow,
    later,
    reanchor,
    windowAround,
    type ItemWindow,
    type Target,
  } from '../lib/window';
  import ItemView from './ItemView.svelte';
  import { whenVisible } from './visible';

  let {
    conversation,
    coverage,
    limits,
    target,
    filter = 'all',
    matches = new Set<string>(),
    current,
    live = false,
    following = $bindable(false),
  }: {
    conversation: Conversation;
    coverage: CoverageState;
    limits: Limits;
    /** The event to show: a #event= link's, or a search result's; it must be in conversation. */
    target: Target | undefined;
    filter?: ItemFilter;
    /** The IDs of the items the search matches. */
    matches?: ReadonlySet<string>;
    /** The ID of the search result shown. */
    current?: string | undefined;
    /** Whether new items may still arrive: the follow control is offered only then. */
    live?: boolean;
    /** Whether the view keeps to the end as items arrive; the reader scrolling up pauses it. */
    following?: boolean;
  } = $props();

  /** How close to the viewport's bottom the end must be for the reader to be at it. */
  const AT_END_SLACK_PX = 48;
  /** How long an item brought into view is kept there while the items around it render. */
  const PIN_SETTLE_MS = 2000;
  /** The keys that scroll the page: pressing one is the reader taking over. */
  const SCROLL_KEYS = new Set(['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' ']);

  const targetId = $derived.by(() => {
    if (target === undefined) return undefined;
    return conversation.items[conversation.itemOf.get(target.event) ?? -1]?.id;
  });
  // Whether the attempt may still grow: its last item can be the model thinking.
  const growing = $derived(live && coverage === 'following');
  const lastId = $derived(conversation.items.at(-1)?.id);
  const visible: readonly Item[] = $derived.by(() => {
    const items = shownItems(conversation.items, growing, targetId);
    return filter === 'all' ? items : items.filter((i) => passes(i, filter));
  });
  const total = $derived(visible.length);
  const positionOf = $derived(new Map(visible.map((item, i) => [item.id, i])));

  // A running attempt opens at its end, following; a finished one at its start.
  following = untrack(
    () => target === undefined && (coverage === 'following' || coverage === 'pending'),
  );
  let range: ItemWindow = $state(
    untrack(() => (following ? lastWindow(total) : firstWindow(total))),
  );
  /** How many items the reader has seen arrive: those after are new. */
  let seen = $state(untrack(() => total));
  const unread = $derived(Math.max(0, total - seen));
  const shown = $derived(visible.slice(range.start, range.end));
  let tail: HTMLElement | undefined = $state();
  let lastScrollY = 0;
  // An element brought into view, and where. Items render at a placeholder's height until they
  // are near the viewport (content-visibility), so those above it grow once it is scrolled to and
  // push it away: it is brought back as the list resizes, until the reader acts or it settles.
  let pin: { element: HTMLElement; block: ScrollLogicalPosition; until: number } | undefined;
  // The IDs rendered, to keep them in place when the list changes; undefined before the first render.
  let shownIds: readonly string[] | undefined;

  // The list changed (an update, a drop, the filter): keep the reader's place, or the end.
  // Another filter is another list: the window restarts around the first item still shown.
  let shownFilter = untrack(() => filter);
  $effect.pre(() => {
    const now = positionOf;
    untrack(() => {
      if (shownIds === undefined) return;
      if (following) {
        range = lastWindow(now.size);
        seen = now.size;
      } else if (filter !== shownFilter) {
        const kept = shownIds.find((id) => now.has(id));
        range = windowAround(kept === undefined ? 0 : (now.get(kept) ?? 0), now.size);
      } else {
        range = reanchor(shownIds, now, now.size);
      }
      shownFilter = filter;
    });
  });

  $effect.pre(() => {
    shownIds = shown.map((item) => item.id);
  });

  // Following: once the DOM has the new items, bring the end into view.
  $effect(() => {
    if (following && shown.length > 0) untrack(scrollToEnd);
  });

  // A new target: show the window around it, stop following, scroll to it.
  $effect(() => {
    const t = target;
    if (t !== undefined) {
      untrack(() => {
        void showTarget(t);
      });
    }
  });

  async function showTarget(t: Target): Promise<void> {
    const item = conversation.items[conversation.itemOf.get(t.event) ?? -1];
    const index = item === undefined ? undefined : positionOf.get(item.id);
    if (item === undefined || index === undefined) return;
    following = false;
    range = windowAround(index, total);
    await tick();
    const element = document.getElementById(anchor(item.id));
    if (element) bringIntoView(element, 'center');
    if (t.focus) element?.focus({ preventScroll: true });
  }

  function bringIntoView(element: HTMLElement, block: ScrollLogicalPosition): void {
    pin = { element, block, until: performance.now() + PIN_SETTLE_MS };
    element.scrollIntoView({ block });
    lastScrollY = window.scrollY;
  }

  function keepPinned(): void {
    if (pin === undefined) return;
    if (!pin.element.isConnected || performance.now() > pin.until) {
      pin = undefined;
      return;
    }
    pin.element.scrollIntoView({ block: pin.block });
    lastScrollY = window.scrollY;
  }

  function unpin(): void {
    pin = undefined;
  }

  function onkeydown(e: KeyboardEvent): void {
    if (SCROLL_KEYS.has(e.key) && !(e.target instanceof HTMLInputElement)) unpin();
  }

  function scrollToEnd(): void {
    tail?.scrollIntoView({ block: 'end' });
    lastScrollY = window.scrollY;
  }

  // Items change size after they render (content-visibility, a result filling a call): while
  // following, every change of the list's size brings the end back into view, else the item
  // just brought into view.
  function keepToEnd(node: HTMLElement): () => void {
    const observer = new ResizeObserver(() => {
      if (following) scrollToEnd();
      else keepPinned();
    });
    observer.observe(node);
    return () => {
      observer.disconnect();
    };
  }

  function atEnd(): boolean {
    return (
      tail !== undefined && tail.getBoundingClientRect().top <= window.innerHeight + AT_END_SLACK_PX
    );
  }

  // Only the reader scrolls up: following scrolls down, and content added below moves nothing.
  function onscroll(): void {
    const y = window.scrollY;
    const up = y < lastScrollY;
    lastScrollY = y;
    if (following && up && !atEnd()) following = false;
  }

  async function follow(): Promise<void> {
    following = true;
    range = lastWindow(total);
    seen = total;
    await tick();
    scrollToEnd();
  }

  function anchor(id: string): string {
    return `event=${id}`;
  }

  function showLater(): void {
    range = later(range, total);
  }

  function showEarlier(): void {
    following = false;
    range = earlier(range);
  }

  function jumpToStart(): void {
    following = false;
    range = firstWindow(total);
  }

  async function jumpToEnd(): Promise<void> {
    range = lastWindow(total);
    await tick();
    if (tail) bringIntoView(tail, 'end');
  }
</script>

<svelte:window {onscroll} {onkeydown} onwheel={unpin} ontouchstart={unpin} onpointerdown={unpin} />

<section class="transcript" aria-label="Transcript">
  <nav class="controls" aria-label="Transcript position">
    <span class="muted" aria-live="polite">
      {#if total === 0}
        {filter === 'all' ? 'No events yet.' : 'No item passes the filter.'}
      {:else}
        Items {formatCount(range.start + 1)}–{formatCount(range.end)} of {formatCount(total)}
      {/if}
    </span>
    {#if total > 0}
      <button type="button" disabled={range.start === 0} onclick={jumpToStart}>Start</button>
      <button type="button" disabled={range.end === total} onclick={jumpToEnd}>End</button>
    {/if}
  </nav>
  {#if range.start > 0}
    <button type="button" class="more" onclick={showEarlier}>
      Show earlier items ({formatCount(range.start)} before these)
    </button>
  {/if}
  <ol class="items" {@attach keepToEnd}>
    {#each shown as item (item.id)}
      {@const targeted = item.id === targetId || item.id === current}
      <li
        id={anchor(item.id)}
        class:targeted
        class:match={matches.has(item.id)}
        class:current={item.id === current}
        tabindex="-1"
      >
        <ItemView
          {item}
          {conversation}
          {coverage}
          {limits}
          {targeted}
          ongoing={growing && item.id === lastId}
        />
      </li>
    {/each}
  </ol>
  {#if range.end < total}
    {#key range.end}
      <div class="sentinel" {@attach whenVisible(showLater)}></div>
    {/key}
    <button type="button" class="more" onclick={showLater}>
      Show later items ({formatCount(total - range.end)} after these)
    </button>
  {/if}
  <div class="tail" bind:this={tail}></div>
  {#if live || unread > 0}
    <div class="follow">
      {#if following}
        <button type="button" aria-pressed="true" onclick={() => (following = false)}>
          Following new items
        </button>
      {:else}
        <button type="button" aria-pressed="false" onclick={follow}>
          Follow new items{unread > 0 ? ` (${formatCount(unread)} new)` : ''}
        </button>
      {/if}
    </div>
  {/if}
</section>

<style>
  .controls {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin: 0 0 0.75rem;
  }
  .controls span {
    margin-right: auto;
  }
  .items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .items > li {
    content-visibility: auto;
    contain-intrinsic-size: auto 3rem;
    border-radius: var(--radius);
  }
  .match {
    box-shadow: -4px 0 0 var(--focus);
  }
  .targeted {
    outline: 2px solid var(--focus);
    outline-offset: 4px;
  }
  .more {
    display: block;
    margin: 0.75rem auto;
  }
  .sentinel,
  .tail {
    height: 1px;
  }
  .follow {
    position: sticky;
    bottom: 1rem;
    display: flex;
    justify-content: flex-end;
    pointer-events: none;
  }
  .follow button {
    pointer-events: auto;
  }
</style>
