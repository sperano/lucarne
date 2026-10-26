import { describe, expect, it } from 'vitest';
import { eventMaker } from '../dev/events';
import { FIXTURE_RUNS } from '../dev/fixtures';
import { buildConversation } from './conversation';

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

  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    'shows every event of %s',
    (_name, fixture) => {
      const attempt = fixture.snapshot.attempts[0]?.id ?? '';
      const c = buildConversation(fixture.snapshot.events, attempt);
      for (const e of fixture.snapshot.events) expect(c.itemOf.has(e.id), e.id).toBe(true);
    },
  );
});
