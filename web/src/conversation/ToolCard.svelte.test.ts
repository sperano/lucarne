import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { CoverageState } from '../lib/contract';
import { buildConversation, type ToolItem } from '../lib/conversation';
import { PREVIEW_LINES } from '../lib/text';
import { ATTEMPT_ID, eventMaker, type EventBody } from '../dev/events';
import { LIMITS } from '../dev/fixtures';
import { render } from '../render';
import ToolCard from './ToolCard.svelte';

const LONG_OUTPUT_LINES = 300;

function toolFrom(bodies: readonly EventBody[]): ToolItem {
  const { items } = buildConversation(bodies.map(eventMaker()), ATTEMPT_ID);
  const [item] = items;
  if (item?.kind !== 'tool') throw new Error('no tool item');
  return item;
}

function tool(): ToolItem {
  const output = Array.from({ length: LONG_OUTPUT_LINES }, (_, i) => `line ${String(i)}`).join(
    '\n',
  );
  return toolFrom([
    {
      kind: 'tool_call',
      toolCall: { callId: 'c', name: 'Bash', input: { command: 'go test ./...' } },
    },
    {
      kind: 'tool_result',
      toolResult: { callId: 'c', status: 'error', exitCode: 1, output },
    },
  ]);
}

function show(item = tool(), coverage: CoverageState = 'ended'): HTMLElement {
  return render(ToolCard, {
    item,
    parentName: undefined,
    coverage,
    limits: LIMITS,
  });
}

describe('ToolCard', () => {
  it('summarizes the call on one line: status, name, command, exit code', () => {
    const summary = show().querySelector('summary');
    expect(summary?.textContent.replace(/\s+/g, ' ').trim()).toBe(
      'error Bash go test ./... exit 1',
    );
  });

  it('shows a background failure reported after an immediate ok result', () => {
    const item = toolFrom([
      { kind: 'tool_call', toolCall: { callId: 'c', name: 'Task' } },
      {
        kind: 'tool_progress',
        toolProgress: { callId: 'c', phase: 'started', background: true },
      },
      {
        kind: 'tool_result',
        toolResult: { callId: 'c', status: 'ok', output: 'running in background' },
      },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'running' } },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'failed' } },
    ]);
    const summary = show(item, 'following').querySelector('summary');
    expect(summary?.querySelector('.status')?.textContent).toBe('failed');
    expect(summary?.textContent).toContain('background');
    expect(summary?.querySelector('.dots')).toBeNull();
  });

  it('opens and closes from the keyboard, with a visible focus ring', async () => {
    const root = show();
    const details = root.querySelector('details');
    await userEvent.keyboard('{Tab}');
    const summary = root.querySelector('summary');
    expect(document.activeElement).toBe(summary);
    expect(summary?.matches(':focus-visible')).toBe(true);
    const ring = summary ? getComputedStyle(summary) : undefined;
    expect(ring?.outlineStyle).toBe('solid');
    expect(ring?.outlineColor).toBe('rgb(9, 105, 218)');
    await userEvent.keyboard('{Enter}');
    expect(details?.open).toBe(true);
    await userEvent.keyboard(' ');
    expect(details?.open).toBe(false);
  });

  it('shows the start of a long output until asked for all of it', async () => {
    const root = show();
    root.querySelector('summary')?.click();
    const pre = () => root.querySelectorAll('pre')[1]?.textContent ?? '';
    expect(pre().split('\n')).toHaveLength(PREVIEW_LINES);
    const button = root.querySelector<HTMLButtonElement>('.output button');
    expect(button?.getAttribute('aria-expanded')).toBe('false');
    button?.focus();
    await userEvent.keyboard('{Enter}');
    expect(pre().split('\n')).toHaveLength(LONG_OUTPUT_LINES);
    expect(button?.getAttribute('aria-expanded')).toBe('true');
  });
});
