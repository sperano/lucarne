<script lang="ts">
  import { formatCount } from '../lib/format';
  import { FILTER_LABELS, ITEM_FILTERS, type ItemFilter } from '../lib/search';

  let {
    query = $bindable(''),
    filter = $bindable('all'),
    matches,
    current,
    scope,
    onstep,
  }: {
    query?: string;
    filter?: ItemFilter;
    /** How many items match the query. */
    matches: number;
    /** The 0-based position of the result shown among the matches, if one is. */
    current: number | undefined;
    /** What the search looks in, said next to it. */
    scope: string;
    /** Shows the next (1) or the previous (-1) result. */
    onstep: (direction: 1 | -1) => void;
  } = $props();

  const countText = $derived.by(() => {
    if (query.trim() === '') return '';
    if (matches === 0) return 'No match';
    if (current === undefined)
      return `${formatCount(matches)} ${matches === 1 ? 'match' : 'matches'}`;
    return `${formatCount(current + 1)} of ${formatCount(matches)}`;
  });

  function onkeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    onstep(event.shiftKey ? -1 : 1);
  }
</script>

<div class="finder" role="search">
  <label class="field">
    <span class="visually-hidden">Search the transcript</span>
    <input type="search" placeholder="Search the transcript" bind:value={query} {onkeydown} />
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
  <label class="field">
    Show
    <select bind:value={filter}>
      {#each ITEM_FILTERS as f (f)}
        <option value={f}>{FILTER_LABELS[f]}</option>
      {/each}
    </select>
  </label>
  <p class="scope muted">{scope}</p>
</div>

<style>
  .finder {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
    font-size: 0.875rem;
  }
  .field {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  input,
  select {
    font: inherit;
  }
  input {
    width: 18rem;
  }
  .count {
    min-width: 7rem;
    min-height: 1lh;
    margin-right: auto;
  }
  .scope {
    flex-basis: 100%;
    margin: 0;
  }
</style>
