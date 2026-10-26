<script lang="ts">
  import Conversation from './conversation/Conversation.svelte';
  import type { RunKey } from './lib/contract';
  import { LiveRun, type LivePage, type LoadFailure, type OpenStream } from './lib/live';
  import { runHref, type View } from './lib/route';
  import { signIn, tabStorage, type Place } from './lib/signin';

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
  // Starts from the link; the attempt picker changes it and the address with it.
  let shownAttempt: string | undefined = $derived(attempt);
  let shownEvent: string | undefined = $derived(event);
  // Whether the page offers to sign in rather than going on its own.
  let signInRefused = $state(false);

  $effect(() => {
    if (view === 'inspector') return;
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
    history.replaceState(null, '', runHref(key, { attempt: id, view }));
  }

  function failureText(failure: LoadFailure): string {
    const code = failure.code === undefined ? '' : ` ${failure.code}`;
    return failure.status === undefined
      ? `The server could not be reached: ${failure.message}`
      : `The run could not be loaded (${String(failure.status)}${code}): ${failure.message}`;
  }
</script>

{#if view === 'inspector'}
  <p class="notice info" role="note">
    The inspector is not built yet. <a href={runHref(key, { view: 'conversation' })}
      >Open the conversation</a
    >.
  </p>
{:else if page.phase === 'loading'}
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
