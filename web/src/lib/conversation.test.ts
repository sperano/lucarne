import { describe, expect, it } from 'vitest';
import { eventMaker } from '../dev/events';
import { FIXTURE_RUNS } from '../dev/fixtures';
import { ConversationBuilder, buildConversation, shownItems } from './conversation';

const ATTEMPT = '7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.codex.0';
const OTHER = '9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b.codex.0';

const ev = eventMaker(ATTEMPT);
const other = eventMaker(OTHER);

const call = (callId: string, name = 'command_execution', extra = {}) =>
  ev({ kind: 'tool_call', toolCall: { callId, name, input: { command: 'go test' }, ...extra } });
const result = (callId: string) =>
  ev({ kind: 'tool_result', toolResult: { callId, status: 'ok' } });
const edit = (callId?: string) =>
  ev({
    kind: 'edit',
    edit: { path: '/workspace/repo/a.go', change: 'update', ...(callId ? { callId } : {}) },
  });

const thinking = () =>
  ev({ kind: 'lifecycle', lifecycle: { phase: 'thinking', sessionId: 'session-example' } });
const say = (text: string) =>
  ev({ kind: 'message', message: { role: 'assistant', type: 'text', text } });

describe('buildConversation', () => {
  it('groups a call with its result and edits, keeping the call position', () => {
    const events = [
      call('c1'),
      ev({ kind: 'message', message: { role: 'assistant', type: 'text', text: 'hi' } }),
      result('c1'),
      edit('c1'),
    ];
    const c = buildConversation(events, ATTEMPT);
    expect(c.items.map((i) => i.kind)).toEqual(['tool', 'event']);
    const tool = c.items[0];
    expect(tool?.kind === 'tool' && tool.result?.id).toBe(events[2]?.id);
    expect(tool?.kind === 'tool' && tool.edits.map((e) => e.id)).toEqual([events[3]?.id]);
    for (const e of events) expect(c.itemOf.has(e.id)).toBe(true);
    expect(c.itemOf.get(events[2]?.id ?? '')).toBe(0);
  });

  it('lets a repeated callId supersede the input in place', () => {
    const first = call('todo', 'todo_list');
    const second = call('todo', 'todo_list', { input: { items: [] } });
    const c = buildConversation([first, second], ATTEMPT);
    expect(c.items).toHaveLength(1);
    const tool = c.items[0];
    expect(tool?.kind === 'tool' && tool.call.id).toBe(second.id);
    expect(tool?.kind === 'tool' && tool.revisions).toBe(1);
    expect(c.itemOf.get(second.id)).toBe(0);
  });

  it('keeps results and edits it cannot group as items of their own', () => {
    const events = [
      result('nowhere'),
      edit(),
      edit('nowhere'),
      call('c1'),
      result('c1'),
      result('c1'),
    ];
    const c = buildConversation(events, ATTEMPT);
    expect(c.items.map((i) => (i.kind === 'tool' ? 'tool' : i.event.kind))).toEqual([
      'tool_result',
      'edit',
      'edit',
      'tool',
      'tool_result',
    ]);
  });

  it('nests sub-calls under the call that made them', () => {
    const c = buildConversation(
      [
        call('task', 'Task'),
        call('read', 'Read', { parentCallId: 'task' }),
        call('x', 'Read', { parentCallId: 'gone' }),
      ],
      ATTEMPT,
    );
    expect(c.items.map((i) => (i.kind === 'tool' ? i.depth : -1))).toEqual([0, 1, 0]);
  });

  it('shows only the selected attempt', () => {
    const c = buildConversation(
      [
        call('c1'),
        call('c2', 'x', {}),
        other({ kind: 'raw', raw: { reason: 'text', text: 'x', bytes: 1 } }),
      ],
      ATTEMPT,
    );
    expect(c.items).toHaveLength(2);
  });

  it('keeps every event of the attempt in order, grouped or not', () => {
    const events = [call('c1'), result('c1'), edit('c1'), result('c1')];
    const c = buildConversation(
      [...events, other({ kind: 'gap', gap: { reason: 'input_limit' } })],
      ATTEMPT,
    );
    expect(c.events).toEqual(events);
    expect(c.items).toHaveLength(2);
  });

  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    'shows every event of %s',
    (_name, fixture) => {
      const attempt = fixture.snapshot.attempts[0]?.id ?? '';
      const c = buildConversation(fixture.snapshot.events, attempt);
      for (const e of fixture.snapshot.events) expect(c.itemOf.has(e.id), e.id).toBe(true);
    },
  );
});

describe('ConversationBuilder', () => {
  it('replaces a tool item when an event joins it, leaving the others as they were', () => {
    const builder = new ConversationBuilder();
    const message = ev({
      kind: 'message',
      message: { role: 'assistant', type: 'text', text: 'hi' },
    });
    builder.add(call('c9'));
    builder.add(message);
    const before = builder.view();
    expect(builder.view()).toBe(before);
    builder.add(result('c9'));
    const after = builder.view();
    expect(after).not.toBe(before);
    expect(after.items[0]).not.toBe(before.items[0]);
    expect(after.items[1]).toBe(before.items[1]);
    const [tool] = before.items;
    expect(tool?.kind === 'tool' && tool.result).toBeUndefined();
    expect(after.calls.get('c9')).toBe(after.items[0]);
  });
});

describe('thinking progress', () => {
  it('makes one item of a batch, showing its latest event', () => {
    const batch = [thinking(), thinking(), thinking()];
    const events = [say('before'), ...batch, say('after'), thinking()];
    const c = buildConversation(events, ATTEMPT);
    expect(c.items.map((i) => i.id)).toEqual([
      events[0]?.id,
      batch[0]?.id,
      events[4]?.id,
      events[5]?.id,
    ]);
    const item = c.items[1];
    expect(item?.kind === 'event' && item.event.id).toBe(batch[2]?.id);
    for (const e of batch) expect(c.itemOf.get(e.id)).toBe(1);
    expect(c.events).toHaveLength(events.length);
  });

  it('is shown only as the last item of an attempt still growing, or when kept', () => {
    const early = thinking();
    const c = buildConversation([early, say('reply'), thinking()], ATTEMPT);
    const [first, reply, last] = c.items;
    expect(shownItems(c.items, true)).toEqual([reply, last]);
    expect(shownItems(c.items, false)).toEqual([reply]);
    expect(shownItems(c.items, false, early.id)).toEqual([first, reply]);
  });
});
