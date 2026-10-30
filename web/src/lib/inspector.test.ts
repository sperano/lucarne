import { describe, expect, it } from 'vitest';
import { eventMaker } from '../dev/events';
import type { Media } from './contract';
import { FIXTURE_RUNS } from '../dev/fixtures';
import { buildConversation } from './conversation';
import {
  PAGE_ROWS,
  SUMMARY_CHARS,
  keepEnd,
  moveSelection,
  reveal,
  rowOf,
  sessionFacts,
} from './inspector';
import { WINDOW_MAX, WINDOW_STEP } from './window';

const ATTEMPT = '7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0';

describe('rowOf', () => {
  const make = eventMaker(ATTEMPT);
  const call = make({
    kind: 'tool_call',
    toolCall: { callId: 'c1', name: 'Bash', input: { command: 'go test ./...\nmore' } },
  });
  const failure = make({
    kind: 'tool_result',
    toolResult: { callId: 'c1', status: 'error', output: 'FAIL\tgreet', exitCode: 1 },
  });
  const pending = make({ kind: 'tool_call', toolCall: { callId: 'c2', name: 'Read' } });
  const progressing = make({ kind: 'tool_call', toolCall: { callId: 'c3', name: 'Bash' } });
  const completed = make({
    kind: 'tool_progress',
    toolProgress: { callId: 'c3', phase: 'completed', elapsedMs: 30000 },
  });
  const backgroundCall = make({
    kind: 'tool_call',
    toolCall: { callId: 'c4', name: 'Task' },
  });
  const backgroundStarted = make({
    kind: 'tool_progress',
    toolProgress: { callId: 'c4', phase: 'started', background: true },
  });
  const backgroundResult = make({
    kind: 'tool_result',
    toolResult: { callId: 'c4', status: 'ok', output: 'running in background' },
  });
  const backgroundFailed = make({
    kind: 'tool_progress',
    toolProgress: { callId: 'c4', phase: 'failed' },
  });
  const long = make({
    kind: 'message',
    message: { role: 'assistant', type: 'text', text: 'x'.repeat(SUMMARY_CHARS * 4) },
  });
  const events = [
    call,
    failure,
    pending,
    progressing,
    completed,
    backgroundCall,
    backgroundStarted,
    backgroundResult,
    backgroundFailed,
    long,
  ];
  const conversation = buildConversation(events, ATTEMPT);

  it('marks a failed call and its result, with the exit code', () => {
    expect(rowOf(call, conversation, 'ended')).toEqual({
      kind: 'Tool call',
      summary: 'Bash go test ./...',
      status: { label: 'error, exit 1', tone: 'error' },
      failed: true,
    });
    expect(rowOf(failure, conversation, 'ended')).toMatchObject({
      kind: 'Tool result',
      summary: 'Bash: FAIL\tgreet',
      failed: true,
    });
  });

  it('says a call has no result yet only while the log is followed', () => {
    expect(rowOf(pending, conversation, 'following').status?.label).toBe('running');
    expect(rowOf(pending, conversation, 'ended').status?.label).toBe('no result');
  });

  it('uses terminal progress when the runner emits no tool result', () => {
    expect(rowOf(progressing, conversation, 'ended').status).toEqual({
      label: 'completed',
      tone: 'ok',
    });
    expect(rowOf(completed, conversation, 'ended')).toMatchObject({
      kind: 'Tool progress',
      summary: 'completed',
      failed: false,
    });
  });

  it('uses a later background failure instead of the immediate ok result', () => {
    expect(rowOf(backgroundCall, conversation, 'following')).toMatchObject({
      status: { label: 'failed', tone: 'error' },
      failed: true,
    });
  });

  it('summarizes a result without output by its media, or says it has none', () => {
    const read = make({ kind: 'tool_call', toolCall: { callId: 'c5', name: 'Read' } });
    const picture = make({
      kind: 'tool_result',
      toolResult: { callId: 'c5', status: 'ok', media: [{ type: 'image' }, { type: 'image' }] },
    });
    const empty = make({ kind: 'tool_result', toolResult: { callId: 'c6', status: 'ok' } });
    const withMedia = buildConversation([read, picture, empty], ATTEMPT);
    expect(rowOf(picture, withMedia, 'ended').summary).toBe('Read: 2 images');
    expect(rowOf(empty, withMedia, 'ended').summary).toBe('call c6: no output');
  });

  it('summarizes a message with the media that came with it', () => {
    const summary = (text: string, media: readonly Media[] = []) =>
      rowOf(
        make({
          kind: 'message',
          message: { role: 'user', type: 'text', text, ...(media.length > 0 && { media }) },
        }),
        conversation,
        'ended',
      ).summary;
    expect(summary('Look\nhere', [{ type: 'image' }])).toBe('User: Look (1 image)');
    expect(summary('', [{ type: 'image' }, { type: 'document' }])).toBe(
      'User: 1 image, 1 document',
    );
    expect(summary('Plain')).toBe('User: Plain');
  });

  it('cuts a long summary', () => {
    const summary = rowOf(long, conversation, 'ended').summary;
    expect(summary).toHaveLength(SUMMARY_CHARS);
    expect(summary.endsWith('…')).toBe(true);
  });

  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    'summarizes every event of %s',
    (_name, { snapshot }) => {
      const c = buildConversation(snapshot.events, snapshot.attempts[0]?.id ?? '');
      for (const event of c.events) {
        const row = rowOf(event, c, 'ended');
        expect(row.kind).not.toBe('');
        expect(row.summary.length).toBeLessThanOrEqual(SUMMARY_CHARS);
      }
    },
  );
});

describe('sessionFacts', () => {
  it('takes the latest model, session and session usage the log reported', () => {
    const make = eventMaker(ATTEMPT);
    const facts = sessionFacts([
      make({
        kind: 'lifecycle',
        lifecycle: { phase: 'session_started', model: 'a', sessionId: 's' },
      }),
      make({ kind: 'usage', usage: { scope: 'turn', inputTokens: 1 } }),
      make({ kind: 'lifecycle', lifecycle: { phase: 'turn_started', model: 'b' } }),
      make({ kind: 'usage', usage: { scope: 'session', inputTokens: 9 } }),
    ]);
    expect(facts).toEqual({
      model: 'b',
      sessionId: 's',
      usage: { scope: 'session', inputTokens: 9 },
      partialUsages: 1,
    });
  });

  it('reports nothing the log does not have', () => {
    expect(sessionFacts([])).toEqual({ partialUsages: 0 });
  });
});

describe('moveSelection', () => {
  const total = 50;
  it.each([
    ['ArrowDown', undefined, 0],
    ['ArrowDown', 3, 4],
    ['ArrowDown', total - 1, total - 1],
    ['ArrowUp', undefined, total - 1],
    ['ArrowUp', 0, 0],
    ['PageDown', 3, 3 + PAGE_ROWS],
    ['PageDown', total - 2, total - 1],
    ['PageUp', 3, 0],
    ['Home', 30, 0],
    ['End', 3, total - 1],
    ['a', 3, undefined],
  ] as const)('%s from %s goes to %s', (key, from, to) => {
    expect(moveSelection(key, from, total)).toBe(to);
  });

  it('moves nowhere in an empty list', () => {
    expect(moveSelection('ArrowDown', undefined, 0)).toBeUndefined();
  });
});

describe('reveal and keepEnd', () => {
  const total = 2000;
  const w = { start: 100, end: 200 };

  it('keeps, grows or moves the window to show a row', () => {
    expect(reveal(w, 150, total)).toBe(w);
    expect(reveal(w, 200, total)).toEqual({ start: 100, end: 200 + WINDOW_STEP });
    expect(reveal(w, 99, total)).toEqual({ start: 0, end: 200 });
    expect(reveal(w, 1000, total)).toEqual({
      start: 1000 - WINDOW_STEP / 2,
      end: 1000 + WINDOW_STEP / 2,
    });
  });

  it('grows to the new end only when it showed the end', () => {
    expect(keepEnd(w, false, total)).toBe(w);
    expect(keepEnd({ start: 0, end: 200 }, true, 250)).toEqual({ start: 0, end: 250 });
    expect(keepEnd({ start: 0, end: 200 }, true, WINDOW_MAX + 50)).toEqual({
      start: 50,
      end: WINDOW_MAX + 50,
    });
  });
});
