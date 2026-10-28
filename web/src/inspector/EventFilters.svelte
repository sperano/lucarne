<script lang="ts">
  import type { EventKind } from '../lib/contract';
  import { filtering, type EventFilter } from '../lib/filters';
  import { formatCount } from '../lib/format';
  import { EVENT_KIND_LABELS } from '../lib/labels';

  let {
    query = $bindable(''),
    filter,
    kinds,
    tools,
    listed,
    total,
    matches,
    current,
    onfilter,
    onstep,
    onreset,
  }: {
    query?: string;
    filter: EventFilter;
    /** The kinds of the attempt's events, with how many of each. */
    kinds: readonly (readonly [EventKind, number])[];
    /** The names of the tools the attempt called. */
    tools: readonly string[];
    /** How many events the filters list, and how many the attempt has. */
    listed: number;
    total: number;
    /** How many listed events match the search. */
    matches: number;
    /** The 0-based position of the selected event among the matches, if it is one. */
    current: number | undefined;
    onfilter: (next: EventFilter) => void;
    /** Selects the next (1) or the previous (-1) match. */
    onstep: (direction: 1 | -1) => void;
    onreset: () => void;
  } = $props();

  const ANY_TOOL = '';

  const countText = $derived.by(() => {
    if (query.trim() === '') return '';
    if (matches === 0) return 'No match';
    if (current === undefined)
      return `${formatCount(matches)} ${matches === 1 ? 'match' : 'matches'}`;
    return `${formatCount(current + 1)} of ${formatCount(matches)}`;
  });
  const active = $derived(filtering(filter) || query !== '');
  // A tool chosen on another attempt stays offered, so the choice reads as it is.
  const toolOptions = $derived(
    filter.tool === undefined || tools.includes(filter.tool) ? tools : [...tools, filter.tool],
  );

  function toggleKind(kind: EventKind, on: boolean): void {
    const rest = filter.kinds.filter((k) => k !== kind);
    onfilter({ ...filter, kinds: on ? [...rest, kind] : rest });
  }

  function chooseTool(e: Event & { currentTarget: HTMLSelectElement }): void {
    const name = e.currentTarget.value;
    onfilter({ ...filter, tool: name === ANY_TOOL ? undefined : name });
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    onstep(event.shiftKey ? -1 : 1);
  }
</script>

<div class="filters" role="search" aria-label="Search and filter the events">
  <div class="row">
    <label>
      <span class="visually-hidden">Search the events</span>
      <input type="search" placeholder="Search the events" bind:value={query} {onkeydown} />
    </label>
    <button
      type="button"
      disabled={matches === 0}
      onclick={() => {
        onstep(-1);
      }}>Previous</button
    >
    <button
      type="button"
      disabled={matches === 0}
      onclick={() => {
        onstep(1);
      }}>Next</button
    >
    <span class="count" aria-live="polite">{countText}</span>
    <button type="button" disabled={!active} onclick={onreset}>Reset</button>
  </div>
  <fieldset class="row">
    <legend class="visually-hidden">Kinds of event listed</legend>
    {#each kinds as [kind, n] (kind)}
      <label class="kind">
        <input
          type="checkbox"
          checked={filter.kinds.includes(kind)}
          onchange={(e) => {
            toggleKind(kind, e.currentTarget.checked);
          }}
        />
        {EVENT_KIND_LABELS[kind]} <span class="muted">{formatCount(n)}</span>
      </label>
    {/each}
  </fieldset>
  <div class="row">
    <label>
      Tool
      <select value={filter.tool ?? ANY_TOOL} onchange={chooseTool}>
        <option value={ANY_TOOL}>Any tool</option>
        {#each toolOptions as name (name)}<option value={name}>{name}</option>{/each}
      </select>
    </label>
    <label>
      <input
        type="checkbox"
        checked={filter.failures}
        onchange={(e) => {
          onfilter({ ...filter, failures: e.currentTarget.checked });
        }}
      />
      Failures only
    </label>
    <span class="muted" aria-live="polite">
      {filtering(filter)
        ? `${formatCount(listed)} of ${formatCount(total)} events listed`
        : `${formatCount(total)} events, none filtered out`}
    </span>
  </div>
</div>

<style>
  .filters {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
    font-size: 0.8125rem;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
    align-items: center;
    margin: 0;
    padding: 0;
    border: 0;
  }
  input,
  select,
  button {
    font: inherit;
  }
  input[type='search'] {
    width: 16rem;
  }
  .count {
    min-width: 6rem;
    min-height: 1lh;
  }
  .kind {
    white-space: nowrap;
  }
</style>
