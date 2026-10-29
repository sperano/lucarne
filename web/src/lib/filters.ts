// The inspector's filters and search over one attempt's events. Both work
// on the events the page holds, one row each: two identical lines are two
// rows, and nothing is merged or hidden but by the reader's own filters.
import { EVENT_KINDS, type EventKind, type TranscriptEvent } from './contract';
import type { Conversation } from './conversation';
import { eventTexts, failedEvent } from './search';

export interface EventFilter {
  /** The kinds listed; none listed means every kind. */
  readonly kinds: readonly EventKind[];
  /** The tool whose calls, results and edits are listed; undefined for every event. */
  readonly tool?: string | undefined;
  /** Only the events that report a failure, and the calls whose result failed. */
  readonly failures: boolean;
}

export const NO_FILTER: EventFilter = { kinds: [], failures: false };

// Events never change (the run store replaces them), so their text is computed once.
const texts = new WeakMap<TranscriptEvent, string>();

/** Whether filter lists less than every event. */
export function filtering(filter: EventFilter): boolean {
  return filter.kinds.length > 0 || filter.tool !== undefined || filter.failures;
}

/** The name of the tool event belongs to: a call's own, or its correlated call's. */
export function toolOf(event: TranscriptEvent, conversation: Conversation): string | undefined {
  switch (event.kind) {
    case 'tool_call':
      return event.toolCall.name;
    case 'tool_result':
      return conversation.calls.get(event.toolResult.callId)?.call.toolCall.name;
    case 'tool_progress':
      return conversation.calls.get(event.toolProgress.callId)?.call.toolCall.name;
    case 'edit':
      return event.edit.callId === undefined
        ? undefined
        : conversation.calls.get(event.edit.callId)?.call.toolCall.name;
    default:
      return undefined;
  }
}

/** Whether event reports a failure, or is a call whose result does. */
export function failedRow(event: TranscriptEvent, conversation: Conversation): boolean {
  if (event.kind !== 'tool_call') return failedEvent(event);
  const tool = conversation.calls.get(event.toolCall.callId);
  if (tool?.result !== undefined) return failedEvent(tool.result);
  const phase = tool?.progress.at(-1)?.toolProgress.phase;
  return phase === 'failed' || phase === 'killed';
}

/** The events of conversation that filter lists, in order. */
export function filterEvents(
  conversation: Conversation,
  filter: EventFilter,
): readonly TranscriptEvent[] {
  if (!filtering(filter)) return conversation.events;
  const kinds = new Set(filter.kinds);
  return conversation.events.filter(
    (event) =>
      (kinds.size === 0 || kinds.has(event.kind)) &&
      (filter.tool === undefined || toolOf(event, conversation) === filter.tool) &&
      (!filter.failures || failedRow(event, conversation)),
  );
}

/** How many events of each kind the attempt has, in the contract's order of kinds; absent kinds left out. */
export function kindCounts(events: readonly TranscriptEvent[]): [EventKind, number][] {
  const counts = new Map<EventKind, number>();
  for (const event of events) counts.set(event.kind, (counts.get(event.kind) ?? 0) + 1);
  return EVENT_KINDS.flatMap((kind) => {
    const n = counts.get(kind);
    return n === undefined ? [] : [[kind, n] as [EventKind, number]];
  });
}

/** The names of the tools the attempt called, sorted. */
export function toolNames(conversation: Conversation): string[] {
  const names = new Set<string>();
  for (const tool of conversation.calls.values()) names.add(tool.call.toolCall.name);
  return [...names].sort((a, b) => a.localeCompare(b));
}

/** The IDs of the events whose text contains query, ignoring case. */
export function searchEvents(events: readonly TranscriptEvent[], query: string): Set<string> {
  const needle = query.trim().toLowerCase();
  if (needle === '') return new Set();
  return new Set(events.filter((e) => textOf(e).includes(needle)).map((e) => e.id));
}

/** The text a search looks in, lowercased: what the event shows, its call ID and its runner type. */
function textOf(event: TranscriptEvent): string {
  let text = texts.get(event);
  if (text === undefined) {
    text = [...eventTexts(event), callIdOf(event), event.source.type]
      .filter((t) => t !== undefined && t !== '')
      .join('\n')
      .toLowerCase();
    texts.set(event, text);
  }
  return text;
}

/** The call ID event names, if any. */
export function callIdOf(event: TranscriptEvent): string | undefined {
  switch (event.kind) {
    case 'tool_call':
      return event.toolCall.callId;
    case 'tool_result':
      return event.toolResult.callId;
    case 'tool_progress':
      return event.toolProgress.callId;
    case 'edit':
      return event.edit.callId;
    default:
      return undefined;
  }
}

/**
 * The index in listed of the next match (direction 1) or the previous one
 * (-1) from the row at index from, wrapping at the ends; from undefined
 * starts before the first row, or after the last one going back.
 */
export function stepMatch(
  listed: readonly TranscriptEvent[],
  matches: ReadonlySet<string>,
  from: number | undefined,
  direction: 1 | -1,
): number | undefined {
  const total = listed.length;
  if (total === 0 || matches.size === 0) return undefined;
  const start = from ?? (direction === 1 ? -1 : total);
  for (let step = 1; step <= total; step++) {
    const i = (((start + direction * step) % total) + total) % total;
    const event = listed[i];
    if (event !== undefined && matches.has(event.id)) return i;
  }
  return undefined;
}
