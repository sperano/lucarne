import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { TranscriptEvent } from '../lib/contract';
import { buildConversation } from '../lib/conversation';
import { WINDOW_MAX, WINDOW_STEP } from '../lib/window';
import { ATTEMPT_ID, eventMaker } from '../dev/events';
import { LIMITS } from '../dev/fixtures';
import { render } from '../render';
import Transcript from './Transcript.svelte';

// A run at maxEventsPerRun: tool calls with 32 KiB outputs, messages, raw lines.
const EVENTS = LIMITS.maxEventsPerRun;
const OUTPUT = 'ok  \texample.com/widget\t0.004s\n'.repeat(LIMITS.maxToolOutputBytes / 32);
// Generous: a page that rendered every item, or every output in full, takes far longer.
const MOUNT_BUDGET_MS = 2000;

function largeRun(): TranscriptEvent[] {
  const make = eventMaker();
  const events: TranscriptEvent[] = [];
  for (let i = 0; events.length < EVENTS; i++) {
    const callId = `call_${String(i)}`;
    events.push(
      make({
        kind: 'message',
        message: { role: 'assistant', type: 'text', text: `Step ${String(i)}` },
      }),
      make({
        kind: 'tool_call',
        toolCall: { callId, name: 'Bash', input: { command: 'go test ./...' } },
      }),
      make({ kind: 'tool_result', toolResult: { callId, status: 'ok', output: OUTPUT } }),
      make({ kind: 'raw', raw: { reason: 'text', text: `stderr ${String(i)}`, bytes: 10 } }),
    );
  }
  return events;
}

const conversation = buildConversation(largeRun(), ATTEMPT_ID);

function show(target?: string): HTMLElement {
  return render(Transcript, { conversation, coverage: 'ended', limits: LIMITS, target });
}

function rendered(root: HTMLElement): number {
  return root.querySelectorAll('.items > li').length;
}

describe('Transcript of a large run', () => {
  it('renders one window of items, outputs collapsed', () => {
    const started = performance.now();
    const root = show();
    expect(performance.now() - started).toBeLessThan(MOUNT_BUDGET_MS);
    expect(rendered(root)).toBe(WINDOW_STEP);
    expect(root.querySelectorAll('details[open]')).toHaveLength(0);
    expect(root.textContent).toContain(`Items 1–${String(WINDOW_STEP)} of`);
  });

  it('grows as the reader scrolls, never past WINDOW_MAX items', async () => {
    const root = show();
    for (let i = 0; i < WINDOW_MAX / WINDOW_STEP + 2; i++) {
      const before = root.querySelector('.items > li:last-child')?.id;
      root.querySelector('.sentinel')?.scrollIntoView();
      await expect.poll(() => root.querySelector('.items > li:last-child')?.id).not.toBe(before);
      expect(rendered(root)).toBeLessThanOrEqual(WINDOW_MAX);
    }
    expect(rendered(root)).toBe(WINDOW_MAX);
    expect(root.querySelector('.more')?.textContent).toContain('Show earlier items');
  });

  it('jumps to the end and back with the keyboard', async () => {
    const root = show();
    const end = [...root.querySelectorAll('button')].find((b) => b.textContent.trim() === 'End');
    end?.focus();
    await userEvent.keyboard('{Enter}');
    const total = conversation.items.length;
    expect(root.querySelector('.items > li:last-child')?.id).toBe(
      `event=${conversation.items[total - 1]?.id ?? ''}`,
    );
    const earlier = root.querySelector<HTMLButtonElement>('.more');
    earlier?.focus();
    await userEvent.keyboard('{Enter}');
    expect(rendered(root)).toBe(2 * WINDOW_STEP);
  });

  it('opens at a linked event, focused and expanded', async () => {
    const tool = conversation.items.find((item, i) => item.kind === 'tool' && i > EVENTS / 8);
    if (tool?.kind !== 'tool' || !tool.result) throw new Error('no tool item');
    const root = show(tool.result.id);
    const li = document.getElementById(`event=${tool.id}`);
    expect(li).not.toBeNull();
    await expect.poll(() => document.activeElement).toBe(li);
    expect(li?.querySelector('details')?.open).toBe(true);
    const box = li?.getBoundingClientRect();
    expect(box && box.top < window.innerHeight && box.bottom > 0).toBe(true);
    expect(rendered(root)).toBeLessThanOrEqual(WINDOW_STEP);
  });
});
