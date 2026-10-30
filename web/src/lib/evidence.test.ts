import { describe, expect, it } from 'vitest';
import { eventMaker, type EventBody } from '../dev/events';
import { snapshotOf } from '../dev/fixtures';
import type { Attempt, Coverage, Stream, TranscriptEvent } from './contract';
import { buildConversation } from './conversation';
import { callEvidence, correlate, MAX_SUBCALLS } from './evidence';

function setup(bodies: readonly EventBody[]) {
  const events = bodies.map(eventMaker());
  const snapshot = snapshotOf('claude-code', 'example-run', events);
  const [attempt] = snapshot.attempts;
  if (!attempt) throw new Error('no attempt');
  return { events, attempt, conversation: buildConversation(events, attempt.id), snapshot };
}

const call = (callId: string, name: string, parentCallId?: string): EventBody => ({
  kind: 'tool_call',
  toolCall: { callId, name, ...(parentCallId === undefined ? {} : { parentCallId }) },
});
const result = (callId: string, status: 'ok' | 'error' | 'unknown' = 'ok'): EventBody => ({
  kind: 'tool_result',
  toolResult: { callId, status },
});
const progress = (callId: string, phase: 'running' | 'completed' | 'failed'): EventBody => ({
  kind: 'tool_progress',
  toolProgress: { callId, phase },
});

const pick = (events: readonly TranscriptEvent[], i: number) => {
  const event = events[i];
  if (!event) throw new Error(`no event ${String(i)}`);
  return event;
};

describe('correlate', () => {
  it('ties a call to its earlier input, results, edits, parent and sub-calls', () => {
    const { events, conversation } = setup([
      call('task', 'Task'),
      call('todo', 'TodoWrite', 'task'),
      call('todo', 'TodoWrite', 'task'),
      call('sub', 'Bash', 'todo'),
      progress('todo', 'running'),
      result('todo'),
      { kind: 'edit', edit: { callId: 'todo', path: 'TODO.md', change: 'update' } },
      result('todo', 'error'),
      { kind: 'message', message: { role: 'assistant', type: 'text', text: 'unrelated' } },
    ]);
    const c = correlate(pick(events, 5), conversation);
    expect(c.callId).toBe('todo');
    expect(c.related.map((r) => [r.relation, r.event.source.line])).toEqual([
      ['parent', 1],
      ['revision', 2],
      ['call', 3],
      ['subcall', 4],
      ['progress', 5],
      ['result', 6],
      ['edit', 7],
      ['result', 8],
    ]);
  });

  it('names nothing for an event without a call', () => {
    const { events, conversation } = setup([
      { kind: 'message', message: { role: 'user', type: 'text', text: 'hi' } },
    ]);
    expect(correlate(pick(events, 0), conversation)).toEqual({ related: [], moreSubcalls: 0 });
  });

  it('lists a bounded number of sub-calls and counts the rest', () => {
    const EXTRA = 5;
    const subs = Array.from({ length: MAX_SUBCALLS + EXTRA }, (_, i) =>
      call(`sub-${String(i)}`, 'Read', 'task'),
    );
    const { events, conversation } = setup([call('task', 'Task'), ...subs]);
    const c = correlate(pick(events, 0), conversation);
    expect(c.related.filter((r) => r.relation === 'subcall')).toHaveLength(MAX_SUBCALLS);
    expect(c.moreSubcalls).toBe(EXTRA);
  });
});

describe('callEvidence', () => {
  const stream: Stream = { epoch: 'e', lastSeq: 1, droppedEvents: 0 };
  const withCoverage = (attempt: Attempt, coverage: Partial<Coverage>): Attempt => ({
    ...attempt,
    coverage: { ...attempt.coverage, ...coverage },
  });

  it('says nothing of a call with a result that has a status', () => {
    const { events, conversation, attempt } = setup([call('c', 'Bash'), result('c')]);
    expect(callEvidence(correlate(pick(events, 0), conversation), attempt, stream)).toBeUndefined();
  });

  it('reports a terminal progress outcome when no result follows', () => {
    const { events, conversation, attempt } = setup([
      call('c', 'Bash'),
      progress('c', 'completed'),
    ]);
    expect(callEvidence(correlate(pick(events, 1), conversation), attempt, stream)).toBe(
      'The runner reported the call completed, but emitted no tool result.',
    );
  });

  it.each([
    [{ state: 'following' }, 'No result yet: the log is still being read.'],
    [{ state: 'unavailable', reason: 'kubelet refused' }, 'cannot be read now (kubelet refused)'],
    [{ state: 'expired', reason: 'pod deleted' }, 'the log is gone (pod deleted)'],
    [{ state: 'ended', gaps: 2 }, 'it has 2 gaps, and the result may be in what is missing'],
    [{ state: 'ended', truncatedLines: 1 }, 'it has 1 line cut'],
    [{ state: 'ended' }, 'read to its end, with no gap and no line cut, and has no result'],
  ] as const)('says why a call has no result: %o', (coverage, said) => {
    const { events, conversation, attempt } = setup([call('c', 'Bash')]);
    const c = correlate(pick(events, 0), conversation);
    expect(callEvidence(c, withCoverage(attempt, coverage), stream)).toContain(said);
  });

  it('says a result does not say whether the call succeeded', () => {
    const { events, conversation, attempt } = setup([call('c', 'Bash'), result('c', 'unknown')]);
    expect(callEvidence(correlate(pick(events, 1), conversation), attempt, stream)).toBe(
      'The result does not say whether the call succeeded.',
    );
  });

  it('ties a result without its call to the events dropped', () => {
    const { events, conversation, attempt } = setup([result('gone', 'ok')]);
    const c = correlate(pick(events, 0), conversation);
    expect(callEvidence(c, attempt, stream)).toBe('Its call is not in the log as read.');
    expect(callEvidence(c, attempt, { ...stream, droppedEvents: 3 })).toContain(
      'it may be among the 3 oldest events the run dropped',
    );
  });
});
