<script lang="ts">
  import Conversation from './conversation/Conversation.svelte';
  import Inspector from './inspector/Inspector.svelte';
  import type { RunKey } from './lib/contract';
  import { LiveRun, type LivePage, type LoadFailure, type OpenStream } from './lib/live';
  import { runHref, type View } from './lib/route';
  import { signIn, tabStorage, type Place } from './lib/signin';
  import ViewSwitch from './ViewSwitch.svelte';

  let {
    key,
    view,
    attempt,
    event,
    fetcher = fetch,
    openStream,
    place = window.location,
    navigate = (href: string) => {
      window.location.assign(href);
    },
  }: {
    key: RunKey;
    view: View;
    attempt: string | undefined;
    event: string | undefined;
    fetcher?: typeof fetch;
    /** Opens the run's stream; an EventSource by default. */
    openStream?: OpenStream;
    /** The page's address, which signing in again comes back to. */
    place?: Place;
    navigate?: (href: string) => void;
  } = $props();

  const NOT_IMPLEMENTED = 501;

  let page: LivePage = $state.raw({ phase: 'loading' });
  // Start from the link; the view switch, the attempt picker and the inspector's selection
  // change them, and the address with them, without loading the run again.
  let shownView: View = $derived(view);
  let shownAttempt: string | undefined = $derived(attempt);
  let shownEvent: string | undefined = $derived(event);
  // Whether the page offers to sign in rather than going on its own.
  let signInRefused = $state(false);

  $effect(() => {
    const run = new LiveRun({
      key,
      fetcher,
      ...(openStream ? { openStream } : {}),
      onchange: (next) => {
        page = next;
        // Nothing is shown yet: go and sign in at once, back to this address.
        if (next.phase === 'failed' && next.failure.signIn) signInRefused = !startSignIn(false);
      },
    });
    run.start();
    return () => {
      run.stop();
    };
  });

  function startSignIn(force: boolean): boolean {
    return signIn(place, tabStorage(), navigate, { force });
  }

  function selectAttempt(id: string): void {
    shownAttempt = id;
    shownEvent = undefined;
    showAddress();
  }

  function selectEvent(id: string): void {
    shownEvent = id;
    showAddress();
  }

  function switchView(next: View): void {
    shownView = next;
    showAddress();
  }

  function showAddress(): void {
    const href = runHref(key, { attempt: shownAttempt, view: shownView, event: shownEvent });
    history.replaceState(null, '', href);
  }

  function failureText(failure: LoadFailure): string {
    const code = failure.code === undefined ? '' : ` ${failure.code}`;
    return failure.status === undefined
      ? `The server could not be reached: ${failure.message}`
      : `The run could not be loaded (${String(failure.status)}${code}): ${failure.message}`;
  }
</script>

<ViewSwitch
  {key}
  view={shownView}
  attempt={shownAttempt}
  event={shownEvent}
  onswitch={switchView}
/>
{#if page.phase === 'loading'}
  <p class="muted" aria-live="polite">Loading the run…</p>
{:else if page.phase === 'failed'}
  <p class="notice error" role="alert">
    {#if page.failure.signIn}
      {#if signInRefused}
        The server asks to sign in again, and did just before.
        <button type="button" onclick={() => startSignIn(true)}>Sign in again</button>
      {:else}
        Signing in again…
      {/if}
    {:else if page.failure.status === NOT_IMPLEMENTED}
      This server does not serve transcripts yet.
    {:else if page.failure.code === 'run_not_found'}
      This run is not known: it never existed here, or its Task was deleted (finished Tasks are kept
      for 72 hours).
    {:else if page.failure.code === 'wrong_run'}
      The server answered with another run than this link's: nothing of it is shown.
    {:else}
      {failureText(page.failure)}{page.retryInMs === undefined ? '' : '. Trying again shortly.'}
    {/if}
  </p>
{:else if shownView === 'inspector'}
  <Inspector
    view={page.view}
    connection={page.connection}
    attempt={shownAttempt}
    event={shownEvent}
    onselectattempt={selectAttempt}
    onselectevent={selectEvent}
    onsignin={() => startSignIn(true)}
  />
{:else}
  <Conversation
    view={page.view}
    connection={page.connection}
    attempt={shownAttempt}
    event={shownEvent}
    onselectattempt={selectAttempt}
    onsignin={() => startSignIn(true)}
  />
{/if}
