<script lang="ts">
  import type { Run } from '../lib/contract';
  import { formatDuration } from '../lib/format';
  import { END_REASON_LABELS, RELOAD_REASON_LABELS, RUN_PHASE_LABELS } from '../lib/labels';
  import type { Connection, LoadFailure } from '../lib/live';

  let {
    connection,
    run,
    onsignin,
  }: {
    connection: Connection;
    run: Run;
    /** Sends the page to sign in again, keeping its address. */
    onsignin: () => void;
  } = $props();

  function failureText(failure: LoadFailure): string {
    const status = failure.status === undefined ? '' : `${String(failure.status)} `;
    const code = failure.code === undefined ? '' : `${failure.code}: `;
    return `${status}${code}${failure.message}`;
  }
</script>

<div class="stream" role="status" aria-live="polite">
  {#if connection.state === 'connecting'}
    <p class="muted">Connecting to the run's live stream…</p>
  {:else if connection.state === 'catching_up'}
    <p class="muted">Catching up with what happened since the run was loaded…</p>
  {:else if connection.state === 'live'}
    <p class="live">
      <span class="dot" aria-hidden="true"></span> Live: new events appear as the log is read.
    </p>
  {:else if connection.state === 'reconnecting'}
    <p class="notice warning">
      The live stream broke: reconnecting from the last event received. Nothing is lost or repeated.
    </p>
  {:else if connection.state === 'reloading'}
    <p class="notice warning">
      {RELOAD_REASON_LABELS[connection.reason]}, so the run is loaded again; what is shown is
      replaced when it comes.
    </p>
  {:else if connection.state === 'retrying'}
    <p class="notice warning">
      The run could not be reloaded ({failureText(connection.failure)}): trying again in
      {formatDuration(connection.retryInMs)}. What was loaded before is shown.
    </p>
  {:else if connection.state === 'ended'}
    <p class="note">
      {END_REASON_LABELS[connection.reason]}
      {#if connection.reason === 'run_finished'}Task phase: {RUN_PHASE_LABELS[run.phase]}.{/if}
    </p>
  {:else if connection.failure.signIn}
    <p class="notice warning">
      Your session expired: the live stream stopped. What was loaded is shown.
      <button type="button" onclick={onsignin}>Sign in again</button>
    </p>
  {:else if connection.failure.code === 'run_not_found'}
    <p class="notice error">
      This run is no longer on the server: its Task was deleted or expired (finished Tasks are kept
      for 72 hours). What was loaded before is shown; nothing more will arrive.
    </p>
  {:else}
    <p class="notice error">
      The run can no longer be followed ({failureText(connection.failure)}). What was loaded before
      is shown; reload the page to try again.
    </p>
  {/if}
</div>

<style>
  .stream p {
    margin: 0;
    font-size: 0.875rem;
  }
  .live {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .dot {
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 50%;
    background: var(--ok);
  }
  button {
    margin-left: 0.5rem;
  }
</style>
