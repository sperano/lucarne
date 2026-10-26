<script lang="ts">
  import Conversation from './conversation/Conversation.svelte';
  import { loadSnapshot, type SnapshotLoad } from './lib/api';
  import type { RunKey } from './lib/contract';
  import { runHref, type View } from './lib/route';

  let {
    key,
    view,
    attempt,
    event,
    fetcher = fetch,
  }: {
    key: RunKey;
    view: View;
    attempt: string | undefined;
    event: string | undefined;
    fetcher?: typeof fetch;
  } = $props();

  const NOT_IMPLEMENTED = 501;

  let load: SnapshotLoad | 'loading' | Error = $state('loading');
  // Starts from the link; the attempt picker changes it and the address with it.
  let shownAttempt: string | undefined = $derived(attempt);
  let shownEvent: string | undefined = $derived(event);

  $effect(() => {
    if (view === 'inspector') return;
    const controller = new AbortController();
    load = 'loading';
    loadSnapshot(key, fetcher, controller.signal).then(
      (result) => {
        if (!controller.signal.aborted) load = result;
      },
      (err: unknown) => {
        if (!controller.signal.aborted) load = err instanceof Error ? err : new Error(String(err));
      },
    );
    return () => {
      controller.abort();
    };
  });

  function selectAttempt(id: string): void {
    shownAttempt = id;
    shownEvent = undefined;
    history.replaceState(null, '', runHref(key, { attempt: id, view }));
  }
</script>

{#if view === 'inspector'}
  <p class="notice info" role="note">
    The inspector is not built yet. <a href={runHref(key, { view: 'conversation' })}
      >Open the conversation</a
    >.
  </p>
{:else if load === 'loading'}
  <p class="muted" aria-live="polite">Loading the run…</p>
{:else if load instanceof Error}
  <p class="notice error" role="alert">The server could not be reached: {load.message}</p>
{:else if !load.ok}
  <p class="notice error" role="alert">
    {#if load.status === NOT_IMPLEMENTED}
      This server does not serve transcripts yet.
    {:else if load.code === 'run_not_found'}
      This run is not known: it never existed here, or its Task was deleted (finished Tasks are kept
      for 72 hours).
    {:else}
      The run could not be loaded ({load.status}{load.code === undefined ? '' : ` ${load.code}`}): {load.message}
    {/if}
  </p>
{:else}
  <Conversation
    snapshot={load.snapshot}
    attempt={shownAttempt}
    event={shownEvent}
    onselectattempt={selectAttempt}
  />
{/if}
