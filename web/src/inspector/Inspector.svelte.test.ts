import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { Attempt, Snapshot } from '../lib/contract';
import { PAGE_ROWS } from '../lib/inspector';
import type { Connection } from '../lib/live';
import { RunStore } from '../lib/runstore';
import { WINDOW_MAX, WINDOW_STEP } from '../lib/window';
import { ATTEMPT_ID, eventMaker, type EventBody } from '../dev/events';
import { FIXTURE_RUNS, findFixture, snapshotOf } from '../dev/fixtures';
import { render } from '../render';
import Inspector from './Inspector.svelte';

/** Renders snapshot in the inspector; selecting an event selects it, as the page does. */
function show(
  snapshot: Snapshot,
  link: { attempt?: string; event?: string; connection?: Connection } = {},
) {
  const props = $state({
    view: new RunStore(snapshot).view(),
    connection: link.connection,
    attempt: link.attempt,
    event: link.event,
    onselectattempt: (id: string) => {
      props.attempt = id;
      props.event = undefined;
    },
    onselectevent: (id: string) => {
      props.event = id;
    },
  });
  const root = render(Inspector, props);
  return { root, props };
}

function snapshotWith(bodies: readonly EventBody[], attempt: Partial<Attempt> = {}): Snapshot {
  const base = snapshotOf('claude-code', 'example-run', bodies.map(eventMaker()));
  const [first] = base.attempts;
  if (!first) throw new Error('no attempt');
  return { ...base, attempts: [{ ...first, ...attempt }] };
}

const rows = (root: HTMLElement) => [...root.querySelectorAll<HTMLElement>('[role=option]')];
const listbox = (root: HTMLElement) => {
  const list = root.querySelector<HTMLElement>('[role=listbox]');
  if (!list) throw new Error('no event list');
  return list;
};
const selectedRow = (root: HTMLElement) =>
  root.querySelector<HTMLElement>('[role=option][aria-selected=true]');
const detail = (root: HTMLElement) =>
  root.querySelector<HTMLElement>('[aria-label="Event detail"]');
const facts = (root: HTMLElement) =>
  Object.fromEntries(
    [...root.querySelectorAll('.context dt')].map((dt) => [
      dt.textContent.trim(),
      dt.nextElementSibling?.textContent.trim(),
    ]),
  );

describe('Inspector over the runner fixtures', () => {
  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    'lists every event of %s',
    (_name, { snapshot }) => {
      const { root } = show(snapshot);
      expect(rows(root)).toHaveLength(Math.min(snapshot.events.length, WINDOW_STEP));
      expect(root.querySelector('h2')?.textContent).toBe(snapshot.run.taskName);
      expect(root.querySelector('.events')?.textContent).toContain(
        `of ${String(snapshot.events.length)}`,
      );
      expect(root.querySelector('script, iframe, img, object, embed, form')).toBeNull();
    },
  );

  it.each(['claude-code', 'codex', 'opencode'] as const)(
    'expands a failed %s command with its input, output and status',
    async (runner) => {
      const { snapshot } = findFixture(runner, 'tool-failures');
      const { root } = show(snapshot);
      const failed = rows(root).find(
        (r) => r.classList.contains('failed') && r.textContent.includes('Tool call'),
      );
      if (!failed) throw new Error('no failed call');
      await userEvent.click(failed);
      expect(failed.getAttribute('aria-selected')).toBe('true');
      const card = detail(root)?.querySelector('.tool');
      expect(card?.querySelector('details')?.open).toBe(true);
      expect(card?.querySelector('.status')?.textContent).toBe('error');
      const id = failed.getAttribute('data-event') ?? '';
      const call = snapshot.events.find((e) => e.id === id);
      if (call?.kind !== 'tool_call') throw new Error('not a call');
      const result = snapshot.events.find(
        (e) => e.kind === 'tool_result' && e.toolResult.callId === call.toolCall.callId,
      );
      if (result?.kind !== 'tool_result') throw new Error('no result');
      expect(card?.textContent).toContain(call.toolCall.name);
      const output = result.toolResult.output?.split('\n')[0];
      if (output !== undefined) expect(card?.textContent).toContain(output);
    },
  );

  it('shows the runner, model and session usage the log reported', () => {
    const { root } = show(findFixture('claude-code', 'session-basic').snapshot);
    const f = facts(root);
    expect(f.Runner).toBe('Claude Code 2.1.296');
    expect(f.Model).toBe('claude-opus-5-5');
    expect(root.querySelector('[aria-label="Session usage"]')?.textContent).toContain(
      '48,384 cache read',
    );
  });
});

describe('Inspector keyboard', () => {
  const make = eventMaker();
  const say = (i: number) =>
    make({
      kind: 'message',
      message: { role: 'assistant', type: 'text', text: `Step ${String(i)}` },
    });
  const EVENTS = 3 * WINDOW_STEP;
  const snapshot = snapshotOf(
    'claude-code',
    'example-run',
    Array.from({ length: EVENTS }, (_, i) => say(i)),
  );
  const selectedText = (root: HTMLElement) => selectedRow(root)?.textContent ?? '';

  it('moves the selection with the arrows, pages and ends', async () => {
    const { root, props } = show(snapshot);
    listbox(root).focus();
    await userEvent.keyboard('{ArrowDown}');
    expect(props.event).toBe(snapshot.events[0]?.id);
    expect(selectedText(root)).toContain('Step 0');
    expect(listbox(root).getAttribute('aria-activedescendant')).toBe(selectedRow(root)?.id);
    await userEvent.keyboard('{ArrowDown}{ArrowDown}{ArrowUp}');
    expect(selectedText(root)).toContain('Step 1');
    await userEvent.keyboard('{PageDown}');
    expect(selectedText(root)).toContain(`Step ${String(1 + PAGE_ROWS)}`);
    await userEvent.keyboard('{End}');
    expect(props.event).toBe(snapshot.events.at(-1)?.id);
    expect(rows(root).length).toBeLessThanOrEqual(WINDOW_MAX);
    await expect
      .poll(() => {
        const box = selectedRow(root)?.getBoundingClientRect();
        return box !== undefined && box.top < window.innerHeight && box.bottom > 0;
      })
      .toBe(true);
    await userEvent.keyboard('{Home}');
    expect(selectedText(root)).toContain('Step 0');
    expect(detail(root)?.textContent).toContain('Step 0');
  });

  it('goes to the detail with Enter and back with Escape', async () => {
    const { root } = show(snapshot, { event: snapshot.events[5]?.id ?? '' });
    listbox(root).focus();
    await userEvent.keyboard('{Enter}');
    expect(document.activeElement).toBe(detail(root));
    await userEvent.keyboard('{Escape}');
    expect(document.activeElement).toBe(listbox(root));
    await userEvent.keyboard('{ArrowDown}');
    expect(selectedText(root)).toContain('Step 6');
  });

  it('opens at a linked event in a long attempt, its rows bounded', async () => {
    const many = snapshotOf(
      'claude-code',
      'example-run',
      Array.from({ length: 20_000 }, (_, i) => say(i)),
    );
    const linked = many.events[15_000];
    const { root } = show(many, { event: linked?.id ?? '' });
    expect(selectedText(root)).toContain('Step 15000');
    expect(rows(root).length).toBeLessThanOrEqual(WINDOW_STEP);
    await expect
      .poll(() => {
        const box = selectedRow(root)?.getBoundingClientRect();
        return box !== undefined && box.top < window.innerHeight && box.bottom > 0;
      })
      .toBe(true);
  });
});

describe('Inspector states', () => {
  it('says what the source did not report', () => {
    const { root } = show(
      snapshotWith([{ kind: 'raw', raw: { reason: 'text', text: 'hi', bytes: 2 } }]),
    );
    const f = facts(root);
    for (const label of ['Model', 'Session', 'Created', 'Started', 'Completed', 'Finished']) {
      expect(f[label], label).toBe('Unknown');
    }
    expect(root.querySelector('.context')?.textContent).toContain(
      'the log reports no session total',
    );
  });

  it('ties an unknown fact to the events dropped, which may have held it', () => {
    const DROPPED = 1_234;
    const snapshot = snapshotWith([{ kind: 'raw', raw: { reason: 'text', text: 'hi', bytes: 2 } }]);
    const { root } = show({ ...snapshot, stream: { ...snapshot.stream, droppedEvents: DROPPED } });
    const f = facts(root);
    const hedged = 'Unknown in the events held (its event may be among the 1,234 dropped)';
    expect(f.Model).toBe(hedged);
    expect(f.Session).toBe(hedged);
    expect(f.Created, 'a fact not read from events').toBe('Unknown');
    expect(root.querySelector('.context')?.textContent).toContain(
      'the events held report no session total (its event may be among the 1,234 dropped)',
    );
  });

  it('keeps truncation, gaps and an expired log in view', async () => {
    const { root } = show(
      snapshotWith(
        [
          { kind: 'gap', gap: { reason: 'read_interrupted', lines: 12 } },
          {
            kind: 'message',
            message: { role: 'assistant', type: 'text', text: 'cut' },
            truncated: ['text'],
          },
        ],
        {
          coverage: {
            state: 'expired',
            lines: 2,
            gaps: 1,
            truncatedLines: 0,
            reason: 'pod deleted',
          },
        },
      ),
    );
    expect(root.textContent).toContain('The log of this attempt is gone');
    expect(root.textContent).toContain('This transcript is incomplete: 1 gap');
    const [gap, message] = rows(root);
    expect(gap?.textContent).toContain('The log stream broke');
    if (!message) throw new Error('no message row');
    await userEvent.click(message);
    expect(detail(root)?.textContent).toContain('Cut to the size limit by Lucarne: text');
  });

  it('says a call is still running while the log is followed, and the stream state', () => {
    const { root } = show(
      snapshotWith([{ kind: 'tool_call', toolCall: { callId: 'c', name: 'Bash' } }], {
        coverage: { state: 'following', lines: 1, gaps: 0, truncatedLines: 0 },
      }),
      { connection: { state: 'reconnecting' } },
    );
    expect(rows(root)[0]?.querySelector('.status')?.textContent).toBe('running');
    expect(root.querySelector('.stream')?.textContent).toContain('reconnecting');
    expect(root.textContent).toContain('The attempt is still running');
  });

  it('says a run has no attempt yet', () => {
    const base = snapshotWith([]);
    const { root } = show({ ...base, run: { ...base.run, phase: 'pending' }, attempts: [] });
    expect(root.textContent).toContain('This run has no attempt yet');
    expect(root.querySelector('[role=listbox]')).toBeNull();
  });

  it('says a linked event is not in the transcript', () => {
    const { root } = show(snapshotWith([]), { event: `${ATTEMPT_ID}:e3b0c44298fc1c14:9:0` });
    expect(root.querySelector('[role=alert]')?.textContent).toContain('is not in this transcript');
  });

  it('keeps hostile content as text', async () => {
    const text = '<img src=x onerror=alert(1)> [x](javascript:alert(1)) **bold**';
    const { root } = show(
      snapshotWith([{ kind: 'message', message: { role: 'assistant', type: 'text', text } }]),
    );
    const [row] = rows(root);
    if (!row) throw new Error('no row');
    expect(row.textContent).toContain('<img src=x');
    await userEvent.click(row);
    expect(detail(root)?.querySelector('strong')?.textContent).toBe('bold');
    expect(root.querySelector('img, script')).toBeNull();
    for (const a of root.querySelectorAll('a')) expect(a.href).toMatch(/^(https?|mailto):/);
  });

  it('offers a newer attempt instead of switching to it', () => {
    const second = '9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b.claude-code.0';
    const base = snapshotWith([{ kind: 'raw', raw: { reason: 'text', text: 'one', bytes: 3 } }]);
    const [first] = base.attempts;
    if (!first) throw new Error('no attempt');
    const { root, props } = show(base);
    const later: Snapshot = { ...base, attempts: [first, { ...first, id: second }] };
    props.view = new RunStore(later).view();
    flushSync();
    const offer = [...root.querySelectorAll('button')].find((b) => b.textContent === 'Show it');
    expect(offer).toBeDefined();
    offer?.click();
    flushSync();
    expect(props.attempt).toBe(second);
  });
});
