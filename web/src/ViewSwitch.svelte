<script lang="ts">
  import type { RunKey } from './lib/contract';
  import { runHref, VIEWS, type View } from './lib/route';

  let {
    key,
    view,
    attempt,
    event,
    onswitch,
  }: {
    key: RunKey;
    view: View;
    attempt: string | undefined;
    event: string | undefined;
    /** Switches the page to another view in place, the run and its stream kept. */
    onswitch: (view: View) => void;
  } = $props();

  const VIEW_LABELS: Record<View, string> = {
    conversation: 'Conversation',
    inspector: 'Inspector',
  };

  function onclick(e: MouseEvent, next: View): void {
    // A new tab or window opens the link itself.
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    if (next !== view) onswitch(next);
  }
</script>

<nav class="views" aria-label="View">
  {#each VIEWS as v (v)}
    <a
      href={runHref(key, { attempt, view: v, event })}
      aria-current={v === view ? 'page' : undefined}
      onclick={(e) => {
        onclick(e, v);
      }}>{VIEW_LABELS[v]}</a
    >
  {/each}
</nav>

<style>
  .views {
    display: flex;
    gap: 0.25rem;
    margin: 0 0 1rem;
    border-bottom: 1px solid var(--border);
  }
  a {
    padding: 0.25rem 0.75rem;
    border: 1px solid transparent;
    border-bottom: 0;
    border-radius: var(--radius) var(--radius) 0 0;
    text-decoration: none;
    font-size: 0.875rem;
  }
  a[aria-current='page'] {
    border-color: var(--border);
    background: var(--bg);
    color: var(--text);
    font-weight: 600;
    margin-bottom: -1px;
  }
</style>
