<script lang="ts">
  import { onMount } from 'svelte';
  import { loadRuns, type ApiFailure } from './lib/api';
  import type { Run, RunList } from './lib/contract';
  import { formatCount, formatTime } from './lib/format';
  import { RUN_PHASE_LABELS, RUNNER_LABELS } from './lib/labels';
  import { homeHref, runLink } from './lib/route';
  import { matchRuns, SHOWN_RUNS } from './lib/runsearch';
  import { signIn, tabStorage, type Place } from './lib/signin';

  let {
    query: initialQuery,
    fetcher = fetch,
    place = window.location,
    navigate = (href: string) => {
      window.location.assign(href);
    },
  }: {
    /** The search query the page's address carries. */
    query: string;
    fetcher?: typeof fetch;
    /** The page's address, which signing in again comes back to. */
    place?: Place;
    navigate?: (href: string) => void;
  } = $props();

  const NOT_IMPLEMENTED = 501;

  type Load =
    | { readonly phase: 'loading' }
    | { readonly phase: 'failed'; readonly failure: ApiFailure | { readonly network: string } }
    | { readonly phase: 'loaded'; readonly list: RunList };

  // Starts from the address; typing changes it, and the address with it. The
  // address is not read again: the page is the only one to change it.
  // svelte-ignore state_referenced_locally
  let query = $state(initialQuery);
  let load = $state.raw<Load>({ phase: 'loading' });
  // Whether the page offers to sign in rather than going on its own.
  let signInRefused = $state(false);
  let input: HTMLInputElement | undefined = $state();
  let results: HTMLOListElement | undefined = $state();

  const matches: Run[] = $derived(load.phase === 'loaded' ? matchRuns(load.list.runs, query) : []);
  const shown = $derived(matches.slice(0, SHOWN_RUNS));

  // The load under way, which a new one or leaving the page cancels.
  let loading: AbortController | undefined;

  onMount(() => {
    reload();
    return () => {
      loading?.abort();
    };
  });

  function reload(): void {
    loading?.abort();
    const controller = new AbortController();
    loading = controller;
    load = { phase: 'loading' };
    loadRuns(fetcher, controller.signal).then(
      (answer) => {
        if (controller.signal.aborted) return;
        if (answer.ok) {
          load = { phase: 'loaded', list: answer.list };
          return;
        }
        load = { phase: 'failed', failure: answer };
        // Nothing is shown yet: go and sign in at once, back to this address.
        if (answer.signIn) signInRefused = !startSignIn(false);
      },
      (err: unknown) => {
        if (controller.signal.aborted) return;
        load = { phase: 'failed', failure: { network: String(err) } };
      },
    );
  }

  $effect(() => {
    input?.focus();
  });

  $effect(() => {
    history.replaceState(null, '', homeHref(query.trim()));
  });

  function startSignIn(force: boolean): boolean {
    return signIn(place, tabStorage(), navigate, { force });
  }

  /** Enter in the search box opens the first match. */
  function openFirst(event: SubmitEvent): void {
    event.preventDefault();
    const first = shown[0];
    if (first !== undefined) navigate(runLink(first.key));
  }

  function links(): HTMLAnchorElement[] {
    return [...(results?.querySelectorAll<HTMLAnchorElement>('a') ?? [])];
  }

  /** ↓ from the search box goes to the first match. */
  function fromInput(event: KeyboardEvent): void {
    if (event.key !== 'ArrowDown') return;
    const first = links()[0];
    if (first === undefined) return;
    event.preventDefault();
    first.focus();
  }

  /** ↑/↓ move between the matches; ↑ from the first goes back to the search box. */
  function betweenLinks(event: KeyboardEvent): void {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    const all = links();
    const at = all.findIndex((link) => link === event.currentTarget);
    const next = event.key === 'ArrowDown' ? all[at + 1] : at === 0 ? input : all[at - 1];
    if (next === undefined) return;
    event.preventDefault();
    next.focus();
  }

  function failureText(failure: ApiFailure): string {
    const code = failure.code === undefined ? '' : ` ${failure.code}`;
    return `The runs could not be listed (${String(failure.status)}${code}): ${failure.message}`;
  }
</script>

<section class="search" aria-labelledby="runs-title">
  <div class="title">
    <h2 id="runs-title">Runs</h2>
    <button type="button" onclick={reload} disabled={load.phase === 'loading'}> Reload </button>
  </div>
  <form role="search" onsubmit={openFirst}>
    <label for="run-query">Search the runs</label>
    <input
      id="run-query"
      type="search"
      bind:this={input}
      bind:value={query}
      onkeydown={fromInput}
      placeholder="Task name, namespace, UID, phase or runner"
      autocomplete="off"
      spellcheck="false"
      aria-controls="run-results"
    />
  </form>
  <p class="note">
    Enter opens the first match; ↓ and ↑ move between them. A run's link,
    <code>/runs/&lbrace;cluster&rbrace;/&lbrace;namespace&rbrace;/&lbrace;taskUID&rbrace;</code>,
    opens it directly.
  </p>

  {#if load.phase === 'loading'}
    <p class="muted" aria-live="polite">Loading the runs…</p>
  {:else if load.phase === 'failed'}
    <p class="notice error" role="alert">
      {#if 'network' in load.failure}
        The server could not be reached: {load.failure.network}
      {:else if load.failure.signIn}
        {#if signInRefused}
          The server asks to sign in again, and did just before.
          <button type="button" onclick={() => startSignIn(true)}>Sign in again</button>
        {:else}
          Signing in again…
        {/if}
      {:else if load.failure.status === NOT_IMPLEMENTED}
        This server reads no cluster: it has no run to list.
      {:else}
        {failureText(load.failure)}
      {/if}
    </p>
  {:else}
    <p class="count" aria-live="polite">
      {#if load.list.runs.length === 0}
        No run to list: the allowlisted namespaces hold no Task.
      {:else if matches.length === 0}
        No run matches “{query.trim()}”.
      {:else if query.trim() === ''}
        {formatCount(matches.length)} runs, newest first.
      {:else}
        {formatCount(matches.length)} of {formatCount(load.list.runs.length)} runs match, newest first.
      {/if}
    </p>
    {#if load.list.omittedRuns > 0}
      <p class="notice warning" role="note">
        Only the {formatCount(load.list.runs.length)} newest runs are listed: the {formatCount(
          load.list.omittedRuns,
        )} older ones are left out, and open from their link only.
      </p>
    {/if}
    {#if matches.length > shown.length}
      <p class="notice info" role="note">
        Showing the first {formatCount(shown.length)} matches: refine the search to reach the {formatCount(
          matches.length - shown.length,
        )} others.
      </p>
    {/if}
    <ol id="run-results" class="runs" aria-label="Matching runs" bind:this={results}>
      {#each shown as run (`${run.key.namespace}/${run.key.taskUID}`)}
        <li>
          <a href={runLink(run.key)} onkeydown={betweenLinks}>
            <span class="name">{run.taskName}</span>
            <span class="phase {run.phase}">{RUN_PHASE_LABELS[run.phase]}</span>
            <span class="facts">
              {#if run.runner !== undefined}{RUNNER_LABELS[run.runner]} ·{/if}
              <code>{run.key.namespace}</code>
              {#if run.created !== undefined}· created {formatTime(run.created)}{/if}
            </span>
          </a>
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .search {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .title {
    display: flex;
    gap: 0.75rem;
    align-items: baseline;
  }
  h2 {
    margin: 0;
    font-size: 1.25rem;
  }
  form {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  label {
    font-weight: 600;
  }
  input {
    font: inherit;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.375rem 0.625rem;
  }
  .count {
    margin: 0;
  }
  .runs {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .runs:empty {
    display: none;
  }
  li + li {
    border-top: 1px solid var(--border);
  }
  a {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.75rem;
    align-items: baseline;
    padding: 0.5rem 0.75rem;
    text-decoration: none;
  }
  a:hover,
  a:focus-visible {
    background: var(--surface);
  }
  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .phase {
    font-size: 0.8125rem;
    font-weight: 600;
    padding: 0 0.5rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    color: var(--text-muted);
  }
  .phase.succeeded {
    color: var(--ok);
  }
  .phase.failed {
    color: var(--error);
  }
  .facts {
    color: var(--text-muted);
    font-size: 0.875rem;
  }
</style>
