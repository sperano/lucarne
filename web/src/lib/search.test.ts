import { describe, expect, it } from 'vitest';
import { ATTEMPT_ID, eventMaker } from '../dev/events';
import { buildConversation } from './conversation';
import { passes, search, textOf } from './search';

const make = eventMaker();
const events = [
  make({
    kind: 'message',
    message: { role: 'assistant', type: 'text', text: 'Running the Tests' },
  }),
  make({
    kind: 'tool_call',
    toolCall: { callId: 'c1', name: 'Bash', input: { command: 'go test ./...' } },
  }),
  make({
    kind: 'tool_result',
    toolResult: { callId: 'c1', status: 'error', output: 'FAIL widget' },
  }),
  make({
    kind: 'tool_call',
    toolCall: { callId: 'c2', name: 'Read', input: { file_path: 'a.go' } },
  }),
  make({ kind: 'tool_result', toolResult: { callId: 'c2', status: 'ok', output: 'package a' } }),
  make({ kind: 'error', error: { message: 'rate limited', code: 'overloaded' } }),
  make({ kind: 'raw', raw: { reason: 'text', text: 'Cloning into widget', bytes: 19 } }),
  make({ kind: 'lifecycle', lifecycle: { phase: 'turn_failed', detail: 'boom' } }),
  make({ kind: 'tool_result', toolResult: { callId: 'gone', status: 'declined' } }),
];
const { items } = buildConversation(events, ATTEMPT_ID);
const ids = (indexes: number[]) => indexes.map((i) => items[i]?.id);

describe('search', () => {
  it('finds items by their text, collapsed parts included, ignoring case', () => {
    expect(search(items, 'widget')).toEqual(ids([1, 4]));
    expect(search(items, 'TESTS')).toEqual(ids([0]));
    expect(search(items, 'test')).toEqual(ids([0, 1]));
    expect(search(items, 'overloaded')).toEqual(ids([3]));
  });

  it('finds nothing for an empty query', () => {
    expect(search(items, '  ')).toEqual([]);
  });

  it('computes an item text once', () => {
    const [first] = items;
    if (!first) throw new Error('no item');
    expect(textOf(first)).toBe(textOf(first));
    expect(textOf(first)).toBe('running the tests');
  });
});

describe('passes', () => {
  it('keeps tool calls, or failures', () => {
    expect(items.filter((i) => passes(i, 'all'))).toHaveLength(items.length);
    expect(items.flatMap((i, n) => (passes(i, 'tools') ? [n] : []))).toEqual([1, 2, 6]);
    expect(items.flatMap((i, n) => (passes(i, 'errors') ? [n] : []))).toEqual([1, 3, 5, 6]);
  });

  it('keeps a call whose later background notification failed', () => {
    const background = [
      make({ kind: 'tool_call', toolCall: { callId: 'background', name: 'Task' } }),
      make({
        kind: 'tool_progress',
        toolProgress: { callId: 'background', phase: 'started', background: true },
      }),
      make({
        kind: 'tool_result',
        toolResult: { callId: 'background', status: 'ok' },
      }),
      make({
        kind: 'tool_progress',
        toolProgress: { callId: 'background', phase: 'failed' },
      }),
    ];
    const conversation = buildConversation(background, ATTEMPT_ID);
    expect(conversation.items.some((item) => passes(item, 'errors'))).toBe(true);
  });
});
