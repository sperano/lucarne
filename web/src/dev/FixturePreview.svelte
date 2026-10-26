<script lang="ts">
  import Conversation from '../conversation/Conversation.svelte';
  import { FIXTURE_RUNS } from './fixtures';

  let index = $state(0);
  const fixture = $derived(FIXTURE_RUNS[index]);
</script>

<main>
  <p class="notice info">
    Development preview: the runner fixtures as the adapters normalize them, in fabricated runs.
    <label>
      Fixture
      <select bind:value={index}>
        {#each FIXTURE_RUNS as run, i (i)}
          <option value={i}>{run.runner} / {run.scenario}</option>
        {/each}
      </select>
    </label>
  </p>
  {#if fixture}
    {#key fixture}
      <Conversation
        snapshot={fixture.snapshot}
        attempt={undefined}
        event={undefined}
        onselectattempt={() => {}}
      />
    {/key}
  {/if}
</main>

<style>
  main {
    max-width: var(--column);
    margin: 1rem auto;
    padding: 0 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
</style>
