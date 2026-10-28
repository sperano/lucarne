<script lang="ts">
  import { untrack } from 'svelte';
  import OutputBlock from '../conversation/OutputBlock.svelte';
  import type { SourceLoad } from '../lib/api';
  import type { Limits, SourceLine } from '../lib/contract';
  import { formatBytes, formatCount, utf8Length } from '../lib/format';
  import { isGapId, type SourceCache } from '../lib/sources';
  import CopyButton from './CopyButton.svelte';

  let {
    eventId,
    line,
    sources,
    limits,
    onsignin = () => {},
  }: {
    eventId: string;
    /** The log line the event names. */
    line: number;
    /** Where the lines come from; absent where the run has no server to ask. */
    sources: SourceCache | undefined;
    limits: Limits;
    onsignin?: () => void;
  } = $props();

  /** How long a selection stays before its line is loaded, so that moving through the rows loads none. */
  const SOURCE_DELAY_MS = 150;

  type Shown =
    | { readonly state: 'loading' }
    | { readonly state: 'loaded'; readonly load: SourceLoad }
    | { readonly state: 'unreachable'; readonly message: string };

  let shown: Shown = $state.raw({ state: 'loading' });
  let formatted = $state(true);
  const gap = $derived(isGapId(eventId));

  $effect(() => {
    const id = eventId;
    const cache = sources;
    if (cache === undefined || isGapId(id)) return;
    const kept = untrack(() => cache.kept(id));
    if (kept !== undefined) {
      shown = { state: 'loaded', load: kept };
      return;
    }
    shown = { state: 'loading' };
    const controller = new AbortController();
    const timer = setTimeout(() => {
      cache.get(id, controller.signal).then(
        (load) => {
          if (!controller.signal.aborted) shown = { state: 'loaded', load };
        },
        (err: unknown) => {
          if (!controller.signal.aborted) {
            shown = {
              state: 'unreachable',
              message: err instanceof Error ? err.message : String(err),
            };
          }
        },
      );
    }, SOURCE_DELAY_MS);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  });

  /** The line formatted when it is whole JSON; undefined for text, or a line cut. */
  function pretty(source: SourceLine): string | undefined {
    if (cut(source)) return undefined;
    try {
      return JSON.stringify(JSON.parse(source.text), null, 2);
    } catch {
      return undefined;
    }
  }

  function cut(source: SourceLine): boolean {
    return utf8Length(source.text) < source.bytes;
  }
</script>

<section class="source" aria-label="Source line">
  <h4>Source line {formatCount(line)}</h4>
  {#if gap}
    <p class="note">
      A gap comes from no line of the log: it says lines are missing before line {formatCount(
        line,
      )}.
    </p>
  {:else if sources === undefined}
    <p class="note">No server to ask for the source lines of this run.</p>
  {:else if shown.state === 'loading'}
    <p class="muted" aria-live="polite">Loading the line the runner wrote…</p>
  {:else if shown.state === 'unreachable'}
    <p class="notice error" role="alert">The server could not be reached: {shown.message}</p>
  {:else if shown.load.ok}
    {@const source = shown.load.line}
    {@const json = pretty(source)}
    <p class="note">
      As the runner wrote it, {formatBytes(source.bytes)}{json === undefined ? '' : ', JSON'}.
    </p>
    {#if cut(source)}
      <p class="notice warning" role="note">
        Only the first {formatBytes(utf8Length(source.text))} of this {formatBytes(source.bytes)} line
        are held (the limit is {formatBytes(limits.maxSourceBytes)}): it is shown as written, not
        parsed.
      </p>
    {/if}
    {#if json !== undefined}
      <label class="toggle"><input type="checkbox" bind:checked={formatted} /> Formatted</label>
    {/if}
    <OutputBlock
      label={json !== undefined && formatted ? 'Source JSON, formatted' : 'Source, as written'}
      text={json !== undefined && formatted ? json : source.text}
    />
    <CopyButton label="Copy the source line" text={() => source.text} />
  {:else if shown.load.signIn}
    <p class="notice warning" role="alert">
      The server asks to sign in again. <button type="button" onclick={onsignin}
        >Sign in again</button
      >
    </p>
  {:else if shown.load.code === 'source_not_found'}
    <p class="notice warning" role="note">
      The server no longer holds this line: it keeps the latest lines of the runs it follows, up to {formatBytes(
        limits.maxRunSourceBytes,
      )} a run, and reads none again to answer.
    </p>
  {:else}
    <p class="notice error" role="alert">
      The source line could not be loaded ({String(shown.load.status)}{shown.load.code === undefined
        ? ''
        : ` ${shown.load.code}`}): {shown.load.message}
    </p>
  {/if}
</section>

<style>
  .source {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  h4 {
    margin: 0;
    font-size: 0.875rem;
  }
  p {
    margin: 0;
  }
  .toggle {
    font-size: 0.8125rem;
  }
</style>
