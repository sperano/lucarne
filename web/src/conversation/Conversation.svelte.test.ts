import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { Attempt, Snapshot, TranscriptEvent } from '../lib/contract';
import { buildConversation } from '../lib/conversation';
import { formatCount } from '../lib/format';
import { RunStore } from '../lib/runstore';
import { WINDOW_STEP } from '../lib/window';
import { ATTEMPT_ID, eventMaker, type EventBody } from '../dev/events';
import { FIXTURE_RUNS, findFixture, snapshotOf } from '../dev/fixtures';
import { render } from '../render';
import Conversation from './Conversation.svelte';

function show(snapshot: Snapshot, attempt?: string, event?: string): HTMLElement {
  const view = new RunStore(snapshot).view();
  return render(Conversation, { view, attempt, event, onselectattempt: () => {} });
}

function snapshotWith(bodies: readonly EventBody[], attempt: Partial<Attempt> = {}): Snapshot {
  const make = eventMaker();
  const base = snapshotOf('claude-code', 'example-run', bodies.map(make));
  const [first] = base.attempts;
  if (!first) throw new Error('no attempt');
  return { ...base, attempts: [{ ...first, ...attempt }] };
}

function texts(root: HTMLElement, selector: string): string[] {
  return [...root.querySelectorAll(selector)].map((e) => e.textContent.trim());
}

describe('Conversation over the runner fixtures', () => {
  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    'renders %s',
    (_name, { snapshot }) => {
      const root = show(snapshot);
      const attempt = snapshot.attempts[0]?.id ?? '';
      const conversation = buildConversation(snapshot.events, attempt);
      expect(root.querySelectorAll('.items > li')).toHaveLength(conversation.items.length);
      expect(root.querySelector('h2')?.textContent).toBe(snapshot.run.taskName);
      for (const event of snapshot.events) {
        if (event.kind === 'message') {
          // The fixtures' messages use no Markdown but bold.
          for (const line of event.message.text.replaceAll('**', '').split('\n')) {
            expect(root.textContent).toContain(line);
          }
        }
        if (event.kind === 'tool_call') expect(root.textContent).toContain(event.toolCall.name);
        if (event.kind === 'error') expect(root.textContent).toContain(event.error.message);
      }
      // Log content never becomes markup that loads or runs anything.
      expect(root.querySelector('script, iframe, img, object, embed, form')).toBeNull();
      for (const a of root.querySelectorAll('a')) expect(a.href).toMatch(/^(https?|mailto):/);
    },
  );

  it('shows a Claude Code session: roles, tools, the edit diff, outputs', () => {
    const root = show(findFixture('claude-code', 'session-basic').snapshot);
    expect(texts(root, '.message .role')).toContain('Assistant');
    expect(root.querySelector('.reasoning')?.hasAttribute('open')).toBe(false);
    const read = [...root.querySelectorAll('.tool')].find((t) => t.textContent.includes('Read'));
    expect(read?.querySelector('summary')?.textContent).toContain('/workspace/repo/greet.go');
    expect(read?.querySelector('.status')?.textContent).toBe('ok');
    expect(texts(root, '.diff .add')).toEqual(['+\treturn "Hello, world"']);
    expect(texts(root, '.diff .delete')).toEqual(['-\treturn "Hello"']);
    expect(root.textContent).toContain("Cloning into '/workspace/repo'...");
    const pr = root.querySelector<HTMLAnchorElement>('.outputs a');
    expect(pr?.href).toBe('https://github.com/example/widget/pull/7');
    expect(pr?.rel).toBe('noopener noreferrer nofollow');
  });

  it('shows a Codex to-do list in its latest state and calls with no outcome', () => {
    const root = show(findFixture('codex', 'interleaved').snapshot);
    const todo = [...root.querySelectorAll('.tool')].find((t) =>
      t.textContent.includes('todo_list'),
    );
    expect(todo?.querySelectorAll('.todos .done')).toHaveLength(2);
    expect(todo?.textContent).toContain('The runner updated this call 2 times');
    expect(texts(root, '.tool .status')).toContain('outcome unknown');
  });

  it('shows OpenCode edits, including those the log has no diff for', () => {
    const root = show(findFixture('opencode', 'edits').snapshot);
    expect(root.querySelectorAll('.edit')).toHaveLength(8);
    expect(root.textContent).toContain('The log carries no diff or content for this change.');
    expect(root.textContent).toContain('The diff in the log is empty.');
    expect(root.textContent).toContain('The file is now empty.');
    expect(texts(root, '.edit .change')).toContain('Deleted');
  });

  it('shows Claude tool progress on the tool card instead of unsupported rows', () => {
    const root = show(findFixture('claude-code', 'background-task').snapshot);
    const tool = root.querySelector('.tool');
    expect(tool?.querySelector('summary')?.textContent).toContain('1 min 00 s');
    expect(root.textContent).not.toContain('Event the adapter does not render');
    expect(root.querySelectorAll('details.raw')).toHaveLength(0);
  });

  it('shows failed and declined calls', () => {
    const statuses = texts(show(findFixture('codex', 'tool-failures').snapshot), '.tool .status');
    expect(statuses).toEqual(['error', 'error', 'error', 'declined']);
  });

  it('keeps unrendered lines collapsed, with their reason', () => {
    const root = show(findFixture('claude-code', 'malformed-lines').snapshot);
    const raws = [...root.querySelectorAll<HTMLDetailsElement>('details.raw')];
    expect(raws.length).toBeGreaterThan(0);
    expect(raws.every((d) => !d.open)).toBe(true);
    expect(texts(root, '.raw .label')).toContain('Malformed JSON line');
  });
});

describe('Conversation states', () => {
  it('marks gaps, cuts and an incomplete log', () => {
    const root = show(
      snapshotWith(
        [
          { kind: 'gap', gap: { reason: 'read_interrupted', lines: 12 } },
          {
            kind: 'message',
            message: { role: 'assistant', type: 'text', text: 'cut' },
            truncated: ['text'],
          },
          { kind: 'raw', raw: { reason: 'oversized', text: 'x'.repeat(10), bytes: 2000000 } },
        ],
        { coverage: { state: 'ended', lines: 3, gaps: 1, truncatedLines: 1 } },
      ),
    );
    expect(root.textContent).toContain('12 lines are missing here.');
    expect(root.textContent).toContain('Cut to the size limit by Lucarne: text (64 KiB).');
    expect(root.textContent).toContain('This transcript is incomplete: 1 gap and 1 line cut');
    expect(root.textContent).toContain('Showing the first 10 B of 1.9 MiB.');
  });

  it('says a call without a result is running only while the log is followed', () => {
    const call: EventBody = { kind: 'tool_call', toolCall: { callId: 'c', name: 'Bash' } };
    const following = show(
      snapshotWith([call], {
        coverage: { state: 'following', lines: 1, gaps: 0, truncatedLines: 0 },
      }),
    );
    expect(following.querySelector('.tool .status')?.textContent).toBe('running');
    expect(following.textContent).toContain('The attempt is still running');
    const ended = show(snapshotWith([call]));
    expect(ended.querySelector('.tool .status')?.textContent).toBe('no result');
  });

  it('says when the log expired or events were dropped', () => {
    const base = snapshotWith([], {
      coverage: { state: 'expired', lines: 0, gaps: 0, truncatedLines: 0, reason: 'pod deleted' },
    });
    const root = show({ ...base, stream: { ...base.stream, droppedEvents: 40 } });
    expect(root.textContent).toContain('The log of this attempt is gone');
    expect(root.textContent).toContain('pod deleted');
    expect(root.textContent).toContain('The 40 oldest events of this run were dropped');
  });

  it('says a run has no attempt yet', () => {
    const base = snapshotWith([]);
    const root = show({ ...base, run: { ...base.run, phase: 'running' }, attempts: [] });
    expect(root.textContent).toContain('This run has no attempt yet');
  });

  it('says the pod of a finished run without attempt is gone', () => {
    const base = snapshotWith([]);
    const root = show({ ...base, run: { ...base.run, phase: 'succeeded' }, attempts: [] });
    expect(root.textContent).toContain('This run lists no attempt: its pod is gone');
    expect(root.textContent).not.toContain('has not been created');
  });

  it('refuses a link to an attempt the run does not have', () => {
    const gone = '1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d.claude-code.0';
    const root = show(snapshotWith([]), gone);
    expect(root.querySelector('[role=alert]')?.textContent).toContain(gone);
    expect(root.querySelector('.transcript')).toBeNull();
  });

  it('says a linked event is not in the transcript', () => {
    const stale = `${ATTEMPT_ID}:0123456789abcdef:7:0`;
    const root = show(snapshotWith([]), undefined, stale);
    expect(root.querySelector('[role=alert]')?.textContent).toContain('is not in this transcript');
  });
});

describe('Conversation of several attempts', () => {
  const second = '9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b.claude-code.0';
  function twoAttempts(): Snapshot {
    const first = snapshotWith([{ kind: 'raw', raw: { reason: 'text', text: 'first', bytes: 5 } }]);
    const events: TranscriptEvent[] = [
      ...first.events,
      eventMaker(second)({ kind: 'raw', raw: { reason: 'text', text: 'second', bytes: 6 } }),
    ];
    const [attempt] = first.attempts;
    if (!attempt) throw new Error('no attempt');
    return {
      ...first,
      events,
      attempts: [attempt, { ...attempt, id: second, podName: 'example-b' }],
    };
  }

  it('follows the latest attempt and lets the reader pick another', async () => {
    const picked: string[] = [];
    const root = render(Conversation, {
      view: new RunStore(twoAttempts()).view(),
      attempt: undefined,
      event: undefined,
      onselectattempt: (id: string) => picked.push(id),
    });
    expect(root.querySelector('.items')?.textContent).toContain('second');
    const select = root.querySelector('select');
    if (!select) throw new Error('no attempt picker');
    await userEvent.selectOptions(select, ATTEMPT_ID);
    expect(picked).toEqual([ATTEMPT_ID]);
  });

  it('shows the attempt a linked event belongs to', () => {
    const root = show(twoAttempts(), undefined, `${ATTEMPT_ID}:e3b0c44298fc1c14:1:0`);
    expect(root.querySelector('.items')?.textContent).toContain('first');
    expect(root.querySelector('[role=alert]')).toBeNull();
  });
});

describe('Conversation search and filter', () => {
  const CALL_AT = 2 * WINDOW_STEP;
  function longRun(): Snapshot {
    const bodies: EventBody[] = [];
    for (let i = 0; i < 2 * CALL_AT; i++) {
      bodies.push({
        kind: 'message',
        message: { role: 'assistant', type: 'text', text: `Step ${String(i)}` },
      });
      if (i === CALL_AT) {
        bodies.push(
          {
            kind: 'tool_call',
            toolCall: { callId: 'c', name: 'Bash', input: { command: 'make' } },
          },
          {
            kind: 'tool_result',
            toolResult: { callId: 'c', status: 'error', output: 'Needle in the output' },
          },
        );
      }
    }
    bodies.push({ kind: 'raw', raw: { reason: 'unknown_type', text: '{"needle":1}', bytes: 12 } });
    return snapshotWith(bodies);
  }

  const count = (root: HTMLElement) => root.querySelector('.finder .count')?.textContent.trim();
  const current = (root: HTMLElement) => root.querySelector('.items > li.current');

  it('finds text in collapsed content and moves between results', async () => {
    const root = show(longRun());
    const input = root.querySelector('input[type=search]');
    if (!input) throw new Error('no search field');
    await userEvent.fill(input, 'NEEDLE');
    expect(count(root)).toBe('2 matches');
    expect(root.querySelector('.finder .scope')?.textContent).toContain(
      `Searches the ${formatCount(2 * CALL_AT + 2)} items of this attempt, collapsed content included`,
    );
    await userEvent.keyboard('{Enter}');
    expect(count(root)).toBe('1 of 2');
    await expect
      .poll(() => current(root)?.querySelector<HTMLDetailsElement>('details')?.open)
      .toBe(true);
    expect(current(root)?.textContent).toContain('Needle in the output');
    await userEvent.keyboard('{Enter}');
    expect(count(root)).toBe('2 of 2');
    await expect
      .poll(() => current(root)?.querySelector<HTMLDetailsElement>('details.raw')?.open)
      .toBe(true);
    await userEvent.keyboard('{Enter}');
    expect(count(root)).toBe('1 of 2');
    await userEvent.keyboard('{Shift>}{Enter}{/Shift}');
    expect(count(root)).toBe('2 of 2');
  });

  it('shows only tool calls or errors when asked', async () => {
    const root = show(longRun());
    const select = root.querySelector<HTMLSelectElement>('.finder select');
    if (!select) throw new Error('no filter');
    await userEvent.selectOptions(select, 'errors');
    expect(root.querySelectorAll('.items > li')).toHaveLength(1);
    expect(root.querySelector('.items')?.textContent).toContain('Bash');
    await userEvent.selectOptions(select, 'all');
    expect(root.querySelectorAll('.items > li')).toHaveLength(WINDOW_STEP);
  });
});
