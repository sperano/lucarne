<script lang="ts">
  import { parseRoute } from './lib/route';
  import { unreachable } from './lib/unreachable';
  import RunPage from './RunPage.svelte';

  const route = parseRoute(window.location.pathname, window.location.search, window.location.hash);
</script>

<header class="site">
  <h1>Lucarne</h1>
  <p class="muted">Read-only transcripts of Kelos agent runs.</p>
</header>

<main>
  {#if route.page === 'home'}
    <p>
      Open a run from its link: <code
        >/runs/&lbrace;cluster&rbrace;/&lbrace;namespace&rbrace;/&lbrace;taskUID&rbrace;</code
      >.
    </p>
  {:else if route.page === 'run'}
    <RunPage key={route.key} view={route.view} attempt={route.attempt} event={route.event} />
    <!-- eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- the last page is tested so that unreachable(route) fails to compile when a page is added. -->
  {:else if route.page === 'invalid'}
    <p class="notice error" role="alert">This is not a valid run link ({route.reason}).</p>
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
  .site p {
    margin: 0;
  }
  main {
    padding-bottom: 3rem;
  }
</style>
