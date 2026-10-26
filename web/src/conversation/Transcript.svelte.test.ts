import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { TranscriptEvent } from '../lib/contract';
import { buildConversation } from '../lib/conversation';
import { WINDOW_MAX, WINDOW_STEP } from '../lib/window';
import { ATTEMPT_ID, eventMaker } from '../dev/events';
import { FIXTURE_KEY, LIMITS, snapshotOf } from '../dev/fixtures';
import { fakeServer, runningSnapshot } from '../dev/server';
import { FakeStreams } from '../dev/stream';
import { render } from '../render';
import RunPage from '../RunPage.svelte';
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
  return render(Transcript, {
    conversation,
    coverage: 'ended',
    limits: LIMITS,
    target: target === undefined ? undefined : { event: target, focus: true },
  });
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

describe('Transcript following a live run', () => {
  const STREAMED = 20;
  const make = eventMaker();
  const say = (i: number) =>
    make({
      kind: 'message',
      message: {
        role: 'assistant',
        type: 'text',
        text: `Step ${String(i)}\n\nChecked the widget.`,
      },
    });
  const all = snapshotOf(
    'claude-code',
    'example-run',
    Array.from({ length: 2 * WINDOW_STEP }, (_, i) => say(i)),
  );
  const loaded = 3 * (WINDOW_STEP / 2);

  async function following() {
    const streams = new FakeStreams();
    const root = render(RunPage, {
      key: FIXTURE_KEY,
      view: 'conversation',
      attempt: undefined,
      event: undefined,
      fetcher: fakeServer([runningSnapshot(all, loaded)]).fetcher,
      openStream: streams.open,
    });
    await expect.poll(() => streams.opened.length).toBe(1);
    const stream = streams.last.open().live(loaded);
    let next = loaded;
    const send = (count: number) => {
      for (const event of all.events.slice(next, next + count)) stream.event(event);
      next += count;
    };
    return { root, send };
  }

  const followButton = (root: HTMLElement) =>
    root.querySelector<HTMLButtonElement>('.follow button');
  const atEnd = (root: HTMLElement) => {
    const tail = root.querySelector('.tail');
    return tail !== null && tail.getBoundingClientRect().top <= window.innerHeight + 48;
  };
  const lastItem = (root: HTMLElement) => root.querySelector('.items > li:last-child')?.textContent;

  it('keeps to the end as events arrive', async () => {
    const { root, send } = await following();
    await expect.poll(() => atEnd(root)).toBe(true);
    expect(followButton(root)?.getAttribute('aria-pressed')).toBe('true');
    send(STREAMED);
    await expect.poll(() => lastItem(root)).toContain(`Step ${String(loaded + STREAMED - 1)}`);
    await expect.poll(() => atEnd(root)).toBe(true);
  });

  it('pauses when the reader scrolls up, holds still, and resumes when asked', async () => {
    const { root, send } = await following();
    await expect.poll(() => atEnd(root)).toBe(true);
    window.scrollBy(0, -window.innerHeight * 2);
    await expect.poll(() => followButton(root)?.getAttribute('aria-pressed')).toBe('false');
    const anchor = [...root.querySelectorAll('.items > li')].find(
      (li) => li.getBoundingClientRect().top >= 0,
    );
    if (!anchor) throw new Error('no item in view');
    const top = anchor.getBoundingClientRect().top;
    const scrolled = window.scrollY;

    send(STREAMED);
    await expect.poll(() => followButton(root)?.textContent).toContain(`(${String(STREAMED)} new)`);
    expect(window.scrollY).toBe(scrolled);
    expect(Math.abs(anchor.getBoundingClientRect().top - top)).toBeLessThan(1);
    // The new items wait below the window, offered by a button.
    expect(lastItem(root)).toContain(`Step ${String(loaded - 1)}`);
    expect(root.textContent).toContain(`Show later items (${String(STREAMED)} after these)`);

    followButton(root)?.click();
    await expect.poll(() => lastItem(root)).toContain(`Step ${String(loaded + STREAMED - 1)}`);
    await expect.poll(() => atEnd(root)).toBe(true);
    expect(followButton(root)?.getAttribute('aria-pressed')).toBe('true');
  });
});
