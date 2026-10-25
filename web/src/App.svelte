<script lang="ts">
  import { parseRoute } from './lib/route';
  import { unreachable } from './lib/unreachable';

  const route = parseRoute(window.location.pathname, window.location.search, window.location.hash);
</script>

<header>
  <h1>Lucarne</h1>
  <p>Read-only transcripts of Kelos agent runs.</p>
</header>

<main>
  {#if route.page === 'home'}
    <p>
      Open a run from its link: <code
        >/runs/&lbrace;cluster&rbrace;/&lbrace;namespace&rbrace;/&lbrace;taskUID&rbrace;</code
      >.
    </p>
  {:else if route.page === 'run'}
    <dl>
      <dt>Cluster</dt>
      <dd>{route.key.cluster}</dd>
      <dt>Namespace</dt>
      <dd>{route.key.namespace}</dd>
      <dt>Task UID</dt>
      <dd><code>{route.key.taskUID}</code></dd>
    </dl>
    <p>The transcript view is not built yet.</p>
  {:else if route.page === 'invalid'}
    <p role="alert">This is not a valid run link ({route.reason}).</p>
  {:else}
    {unreachable(route)}
  {/if}
</main>

<style>
  :global(body) {
    font-family: system-ui, sans-serif;
    margin: 2rem;
    color: #1f2328;
  }
  dl {
    display: grid;
    grid-template-columns: max-content auto;
    gap: 0.25rem 1rem;
  }
  dd {
    margin: 0;
  }
</style>
