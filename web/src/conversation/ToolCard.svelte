<script lang="ts">
  import type { CoverageState, Limits } from '../lib/contract';
  import type { ToolItem } from '../lib/conversation';
  import { formatDuration } from '../lib/format';
  import { STATUS_LABELS, TOOL_PROGRESS_LABELS } from '../lib/labels';
  import {
    effectiveToolOutcome,
    latestProgressBackground,
    latestProgressElapsed,
  } from '../lib/tool-progress';
  import { describeTool } from '../lib/tools';
  import EditView from './EditView.svelte';
  import OutputBlock from './OutputBlock.svelte';
  import Pulse from './Pulse.svelte';
  import ToolResult from './ToolResult.svelte';
  import Truncated from './Truncated.svelte';

  let {
    item,
    parentName,
    coverage,
    limits,
    open = false,
  }: {
    item: ToolItem;
    /** The name of the call that made this one, when the transcript has it. */
    parentName: string | undefined;
    coverage: CoverageState;
    limits: Limits;
    open?: boolean;
  } = $props();

  /** Sub-calls indent this many levels at most. */
  const MAX_DEPTH_CLASS = 3;

  const call = $derived(item.call.toolCall);
  const tool = $derived(describeTool(call));
  const result = $derived(item.result?.toolResult);
  const outcome = $derived(effectiveToolOutcome(item));
  // A call without a result or progress is running only while the log is still being read.
  const pendingLabel = $derived(coverage === 'following' ? 'running' : 'no result');
  const status = $derived(
    outcome?.kind === 'tool_result'
      ? outcome.toolResult.status
      : outcome?.kind === 'tool_progress'
        ? outcome.toolProgress.phase
        : 'pending',
  );
  const statusLabel = $derived(
    outcome?.kind === 'tool_result'
      ? STATUS_LABELS[outcome.toolResult.status]
      : outcome?.kind === 'tool_progress'
        ? TOOL_PROGRESS_LABELS[outcome.toolProgress.phase]
        : pendingLabel,
  );
  const background = $derived(latestProgressBackground(item.progress));
  const elapsedMs = $derived(latestProgressElapsed(item.progress));
  const working = $derived(
    coverage === 'following' &&
      outcome?.kind === 'tool_progress' &&
      (outcome.toolProgress.phase === 'started' || outcome.toolProgress.phase === 'running'),
  );
  const depth = $derived(Math.min(item.depth, MAX_DEPTH_CLASS));
</script>

<article class="tool depth-{String(depth)}" aria-label="Tool call {call.name}">
  <details {open}>
    <summary>
      <span class="status {status}">{statusLabel}</span>
      {#if working}<Pulse />{/if}
      <span class="name">{call.name}</span>
      {#if tool.subject !== ''}<code class="subject">{tool.subject}</code>{/if}
      {#if background === true}<span class="muted">background</span>{/if}
      {#if elapsedMs !== undefined}<span class="muted">{formatDuration(elapsedMs)}</span>{/if}
      {#if result?.exitCode !== undefined}<span class="muted">exit {result.exitCode}</span>{/if}
    </summary>
    <div class="body">
      {#if parentName !== undefined}<p class="note">Called from {parentName}.</p>{/if}
      {#if item.revisions > 0}
        <p class="note">
          The runner updated this call {item.revisions === 1
            ? 'once'
            : `${String(item.revisions)} times`}; this is its latest input.
        </p>
      {/if}
      {#if tool.command !== undefined}<OutputBlock
          label="Command"
          prefix="$ "
          text={tool.command}
        />{/if}
      {#if tool.todos}
        <ul class="todos" aria-label="To-do list">
          {#each tool.todos as todo, i (i)}
            <li class:done={todo.done}>
              <span aria-hidden="true">{todo.done ? '☑' : '☐'}</span>
              <span class="visually-hidden">{todo.done ? 'done:' : 'to do:'}</span>
              {todo.text}
            </li>
          {/each}
        </ul>
      {/if}
      {#if tool.inputJSON !== undefined}
        <OutputBlock label="Input" text={tool.inputJSON} />
      {:else if call.inputText !== undefined}
        <OutputBlock label="Input (cut)" text={call.inputText} />
      {/if}
      <Truncated fields={item.call.truncated} {limits} />
      {#if item.result}
        <ToolResult event={item.result} {limits} />
      {:else}
        <p class="note">
          {coverage === 'following'
            ? 'No result in the log yet.'
            : 'The log has no result for this call.'}
        </p>
      {/if}
    </div>
  </details>
  {#each item.edits as edit (edit.id)}<EditView event={edit} {limits} />{/each}
</article>

<style>
  .tool {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.25rem 0.75rem;
    background: var(--bg);
  }
  .depth-1 {
    margin-left: 1.5rem;
  }
  .depth-2 {
    margin-left: 3rem;
  }
  .depth-3 {
    margin-left: 4.5rem;
  }
  summary {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    cursor: pointer;
    padding: 0.125rem 0;
    min-width: 0;
  }
  summary::before {
    content: '▸';
    color: var(--text-muted);
  }
  details[open] > summary::before {
    content: '▾';
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .name {
    font-weight: 600;
  }
  .subject {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1;
  }
  .status {
    font-size: 0.8125rem;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .status.ok {
    color: var(--ok);
  }
  .status.error {
    color: var(--error);
  }
  .status.failed,
  .status.killed {
    color: var(--error);
  }
  .body {
    padding: 0 0 0.5rem;
  }
  .todos {
    list-style: none;
    padding-left: 0;
    margin: 0.5rem 0;
  }
  .done {
    color: var(--text-muted);
  }
</style>
