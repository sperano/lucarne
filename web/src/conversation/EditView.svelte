<script lang="ts">
  import type { Limits } from '../lib/contract';
  import type { EventOf } from '../lib/conversation';
  import { EDIT_LABELS } from '../lib/labels';
  import DiffView from './DiffView.svelte';
  import OutputBlock from './OutputBlock.svelte';
  import Truncated from './Truncated.svelte';

  let { event, limits }: { event: EventOf<'edit'>; limits: Limits } = $props();

  const edit = $derived(event.edit);
</script>

<section class="edit" aria-label="{EDIT_LABELS[edit.change]} {edit.path}">
  <p class="edit-head">
    <span class="change {edit.change}">{EDIT_LABELS[edit.change]}</span>
    <code>{edit.path}</code>
  </p>
  {#if edit.diff !== undefined}
    {#if edit.diff === ''}
      <p class="note">The diff in the log is empty.</p>
    {:else}
      <DiffView diff={edit.diff} />
    {/if}
  {:else if edit.before !== undefined || edit.after !== undefined}
    {#if edit.before !== undefined}<OutputBlock label="Before" text={edit.before} />{/if}
    {#if edit.after !== undefined}
      {#if edit.after === ''}
        <p class="note">The file is now empty.</p>
      {:else}
        <OutputBlock label="After" text={edit.after} />
      {/if}
    {/if}
  {:else}
    <p class="note">The log carries no diff or content for this change.</p>
  {/if}
  <Truncated fields={event.truncated} {limits} />
</section>

<style>
  .edit {
    margin: 0.5rem 0 0;
  }
  .edit-head {
    margin: 0 0 0.25rem;
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
  }
  .change {
    font-weight: 600;
  }
  .add {
    color: var(--ok);
  }
  .delete {
    color: var(--error);
  }
</style>
