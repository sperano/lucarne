<script lang="ts">
  import { parseRoute } from './lib/route';
  import { unreachable } from './lib/unreachable';
  import RunPage from './RunPage.svelte';
  import RunSearch from './RunSearch.svelte';

  const route = parseRoute(window.location.pathname, window.location.search, window.location.hash);
</script>

<header class="site">
  <h1><a href="/" title="Search the runs">Lucarne</a></h1>
  <p class="muted">Read-only transcripts of Kelos agent runs.</p>
</header>

<main>
  {#if route.page === 'home'}
    <RunSearch query={route.query} />
  {:else if route.page === 'run'}
    <RunPage key={route.key} view={route.view} attempt={route.attempt} event={route.event} />
    <!-- eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- the last page is tested so that unreachable(route) fails to compile when a page is added. -->
  {:else if route.page === 'invalid'}
    <p class="notice error" role="alert">
      This is not a valid run link ({route.reason}). <a href="/">Search the runs</a>
    </p>
  {:else}
    {unreachable(route)}
  {/if}
</main>

<style>
  .site,
  main {
    max-width: var(--column);
    margin: 0 auto;
    padding: 0 1.5rem;
  }
  .site {
    display: flex;
    gap: 1rem;
    align-items: baseline;
    padding-top: 1rem;
    padding-bottom: 0.5rem;
    margin-bottom: 1rem;
    border-bottom: 1px solid var(--border);
  }
  h1 {
    margin: 0;
    font-size: 1.125rem;
  }
  h1 a {
    color: inherit;
    text-decoration: none;
  }
  .site p {
    margin: 0;
  }
  main {
    padding-bottom: 3rem;
  }
  /* The inspector's three panes need the width of a desktop screen. */
  .site:has(+ main :global(.inspector)),
  main:has(:global(.inspector)) {
    max-width: var(--wide);
  }
</style>
