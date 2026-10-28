import { describe, expect, it } from 'vitest';
import { eventMaker, type EventBody } from '../dev/events';
import { findFixture } from '../dev/fixtures';
import { buildConversation } from './conversation';
import {
  filterEvents,
  filtering,
  kindCounts,
  NO_FILTER,
  searchEvents,
  stepMatch,
  toolNames,
  toolOf,
} from './filters';

const BODIES: EventBody[] = [
  { kind: 'message', message: { role: 'assistant', type: 'text', text: 'Same line' } },
  { kind: 'message', message: { role: 'assistant', type: 'text', text: 'Same line' } },
  { kind: 'tool_call', toolCall: { callId: 'call-1', name: 'Bash', input: { command: 'make' } } },
  { kind: 'tool_result', toolResult: { callId: 'call-1', status: 'error', exitCode: 2 } },
  { kind: 'tool_call', toolCall: { callId: 'call-2', name: 'Read', input: { path: 'go.mod' } } },
  { kind: 'tool_result', toolResult: { callId: 'call-2', status: 'ok', output: 'module x' } },
  { kind: 'edit', edit: { callId: 'call-2', path: 'go.mod', change: 'update' } },
  { kind: 'raw', raw: { reason: 'unknown_type', text: '{"type":"novel"}', bytes: 16 } },
  { kind: 'gap', gap: { reason: 'read_interrupted' } },
];
const events = BODIES.map(eventMaker());
const conversation = buildConversation(events, events[0]?.attempt ?? '');
const ids = (list: readonly { id: string }[]) => list.map((e) => e.id);
const at = (...indexes: number[]) => indexes.map((i) => eventAt(i).id);

function eventAt(i: number) {
  const event = events[i];
  if (!event) throw new Error(`no event ${String(i)}`);
  return event;
}

describe('filterEvents', () => {
  it('lists every event, identical ones included, without a filter', () => {
    expect(filtering(NO_FILTER)).toBe(false);
    expect(filterEvents(conversation, NO_FILTER)).toBe(conversation.events);
    expect(ids(filterEvents(conversation, NO_FILTER))).toEqual(ids(events));
  });

  it('lists the kinds chosen, both repeated lines among them', () => {
    const f = { ...NO_FILTER, kinds: ['message', 'raw'] as const };
    expect(ids(filterEvents(conversation, { ...f, kinds: [...f.kinds] }))).toEqual(at(0, 1, 7));
  });

  it("lists a tool's calls, results and edits", () => {
    expect(toolOf(eventAt(6), conversation)).toBe('Read');
    expect(ids(filterEvents(conversation, { ...NO_FILTER, tool: 'Read' }))).toEqual(at(4, 5, 6));
  });

  it('lists the failures and the calls whose result failed', () => {
    expect(ids(filterEvents(conversation, { ...NO_FILTER, failures: true }))).toEqual(at(2, 3));
  });

  it('combines the filters', () => {
    const f = { kinds: ['tool_result' as const], tool: 'Bash', failures: true };
    expect(ids(filterEvents(conversation, f))).toEqual(at(3));
  });
});

describe('kinds and tools', () => {
  it('counts the kinds present, in the contract order', () => {
    expect(kindCounts(events)).toEqual([
      ['message', 2],
      ['tool_call', 2],
      ['tool_result', 2],
      ['edit', 1],
      ['raw', 1],
      ['gap', 1],
    ]);
  });

  it('names the tools called, sorted', () => {
    expect(toolNames(conversation)).toEqual(['Bash', 'Read']);
  });
});

describe('searchEvents', () => {
  it('finds both repeated lines, ignoring case', () => {
    expect([...searchEvents(events, 'SAME line')]).toEqual(at(0, 1));
  });

  it('looks in tool input, call IDs and runner types', () => {
    expect([...searchEvents(events, 'go.mod')]).toEqual(at(4, 6));
    expect([...searchEvents(events, 'call-1')]).toEqual(at(2, 3));
    const { snapshot } = findFixture('codex', 'session-basic');
    const typed = searchEvents(snapshot.events, 'item.completed');
    expect(typed.size).toBe(
      snapshot.events.filter((e) => e.source.type === 'item.completed').length,
    );
    expect(typed.size).toBeGreaterThan(0);
  });

  it('finds nothing for a blank query', () => {
    expect(searchEvents(events, '  ').size).toBe(0);
  });
});

describe('stepMatch', () => {
  const matches = new Set(at(1, 4));

  it('goes to the next and the previous match, wrapping', () => {
    expect(stepMatch(events, matches, undefined, 1)).toBe(1);
    expect(stepMatch(events, matches, 1, 1)).toBe(4);
    expect(stepMatch(events, matches, 4, 1)).toBe(1);
    expect(stepMatch(events, matches, undefined, -1)).toBe(4);
    expect(stepMatch(events, matches, 2, -1)).toBe(1);
  });

  it('finds none without a match or a row', () => {
    expect(stepMatch(events, new Set(), 0, 1)).toBeUndefined();
    expect(stepMatch([], matches, undefined, 1)).toBeUndefined();
  });
});
