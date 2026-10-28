<script lang="ts">
  import type { Selection } from '../lib/selection';

  let {
    selection,
    finished,
    live,
    event,
    eventHere,
    found,
  }: {
    selection: Selection;
    /** Whether the run finished: a pod it does not list is gone, not yet to come. */
    finished: boolean;
    /** Whether updates may still arrive. */
    live: boolean;
    /** The event of the link's #event= fragment, if any. */
    event: string | undefined;
    /** Whether that event belongs to the attempt shown. */
    eventHere: boolean;
    /** Whether that event is in the attempt's transcript. */
    found: boolean;
  } = $props();

  const selected = $derived(selection.attempt);
</script>

{#if selection.missing !== undefined}
  <p class="notice error" role="alert">
    This run has no attempt <code>{selection.missing}</code>: the link names a pod this run does not
    have, or no longer lists. Choose one of its attempts above.
  </p>
{:else if !selected && finished}
  <p class="notice info" role="note">
    This run lists no attempt: its pod is gone, deleted or removed with its log, or was never
    created.
  </p>
{:else if !selected}
  <p class="notice info" role="note">
    This run has no attempt yet: its pod has not been created{live
      ? '; it is shown here once it starts'
      : ''}.
  </p>
{/if}
{#if event !== undefined && selected !== undefined && !found}
  <p class="notice warning" role="alert">
    {#if !eventHere}
      The linked event <code>{event}</code> belongs to another attempt than the one shown.
    {:else if live}
      The linked event <code>{event}</code> is not in this transcript yet: it is shown if it arrives.
      The log may have been read again since the link was made, or the run dropped the event to stay within
      its bounds.
    {:else}
      The linked event <code>{event}</code> is not in this transcript: the log was read again since the
      link was made, or the run dropped the event to stay within its bounds.
    {/if}
  </p>
{/if}
