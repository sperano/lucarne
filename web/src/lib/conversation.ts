// The conversation view's model: one attempt's events in order, a tool call
// grouped with its result and the edits it made, a batch of thinking
// progress one item, everything else one item per event. Nothing is
// dropped: an event that cannot be grouped (a result or an edit whose call
// is not in the transcript, a second result for one call) stays an item of
// its own.
//
// Items are immutable: an event that joins a tool item replaces it with a
// new one, so a live view can add events one at a time and the components
// re-render exactly the items that changed.
import type { EventKind, TranscriptEvent } from './contract';

export type EventOf<K extends EventKind> = Extract<TranscriptEvent, { kind: K }>;

export interface ToolItem {
  readonly kind: 'tool';
  /** The ID of the call's first event. */
  readonly id: string;
  /** The latest event of the call: a repeated callId supersedes the input. */
  readonly call: EventOf<'tool_call'>;
  /** How many times a later event replaced the call's input. */
  readonly revisions: number;
  readonly result?: EventOf<'tool_result'>;
  readonly edits: readonly EventOf<'edit'>[];
  /** How many calls enclose this one through parentCallId, as far as the transcript shows. */
  readonly depth: number;
}

export interface EventItem {
  readonly kind: 'event';
  readonly id: string;
  readonly event: Exclude<TranscriptEvent, EventOf<'tool_call'>>;
}

export type Item = ToolItem | EventItem;

export interface Conversation {
  /** The attempt's events in the order they came, each once: what the inspector lists. */
  readonly events: readonly TranscriptEvent[];
  readonly items: readonly Item[];
  /** The index in items of the item that shows each event ID. */
  readonly itemOf: ReadonlyMap<string, number>;
  /** The tool items by callId, to name a sub-call's parent. */
  readonly calls: ReadonlyMap<string, ToolItem>;
}

/** Builds the conversation of attempt from a run's events, in their order. */
export function buildConversation(
  events: readonly TranscriptEvent[],
  attempt: string,
): Conversation {
  const builder = new ConversationBuilder();
  for (const event of events) if (event.attempt === attempt) builder.add(event);
  return builder.view();
}

/** Builds one attempt's conversation an event at a time. */
export class ConversationBuilder {
  private readonly events: TranscriptEvent[] = [];
  private readonly items: Item[] = [];
  private readonly itemOf = new Map<string, number>();
  private readonly calls = new Map<string, ToolItem>();
  private current: Conversation | undefined;

  /** Adds the next event of the attempt. */
  add(event: TranscriptEvent): void {
    this.current = undefined;
    this.events.push(event);
    switch (event.kind) {
      case 'tool_call':
        this.addCall(event);
        return;
      case 'tool_result': {
        const tool = this.calls.get(event.toolResult.callId);
        if (tool && !tool.result) this.replace(tool, { ...tool, result: event }, event.id);
        else this.push({ kind: 'event', id: event.id, event });
        return;
      }
      case 'edit': {
        const callId = event.edit.callId;
        const tool = callId === undefined ? undefined : this.calls.get(callId);
        if (tool) this.replace(tool, { ...tool, edits: [...tool.edits, event] }, event.id);
        else this.push({ kind: 'event', id: event.id, event });
        return;
      }
      case 'lifecycle':
        this.addLifecycle(event);
        return;
      default:
        this.push({ kind: 'event', id: event.id, event });
    }
  }

  /** The conversation so far; the same object until the next add. */
  view(): Conversation {
    this.current ??= {
      events: this.events.slice(),
      items: this.items.slice(),
      itemOf: new Map(this.itemOf),
      calls: new Map(this.calls),
    };
    return this.current;
  }

  private addCall(event: EventOf<'tool_call'>): void {
    const tool = this.calls.get(event.toolCall.callId);
    if (tool) {
      this.replace(tool, { ...tool, call: event, revisions: tool.revisions + 1 }, event.id);
      return;
    }
    const parentId = event.toolCall.parentCallId;
    const parent = parentId === undefined ? undefined : this.calls.get(parentId);
    const created: ToolItem = {
      kind: 'tool',
      id: event.id,
      call: event,
      revisions: 0,
      edits: [],
      depth: parent ? parent.depth + 1 : 0,
    };
    this.calls.set(event.toolCall.callId, created);
    this.push(created);
  }

  /** A thinking event right after another joins its item, which then shows the latest. */
  private addLifecycle(event: EventOf<'lifecycle'>): void {
    const index = this.items.length - 1;
    const last = this.items[index];
    if (isThinking(event) && last?.kind === 'event' && isThinking(last.event)) {
      this.items[index] = { ...last, event };
      this.itemOf.set(event.id, index);
      return;
    }
    this.push({ kind: 'event', id: event.id, event });
  }

  private push(item: Item): void {
    this.itemOf.set(item.id, this.items.length);
    this.items.push(item);
  }

  /** Puts next in the place of tool, which was pushed when its call came, and shows event id in it. */
  private replace(tool: ToolItem, next: ToolItem, id: string): void {
    const index = this.itemOf.get(tool.id);
    if (index === undefined) return;
    this.items[index] = next;
    this.calls.set(next.call.toolCall.callId, next);
    this.itemOf.set(id, index);
  }
}

function isThinking(event: TranscriptEvent): boolean {
  return event.kind === 'lifecycle' && event.lifecycle.phase === 'thinking';
}

/** Whether item is a batch of the runner's thinking progress. */
export function isThinkingItem(item: Item): boolean {
  return item.kind === 'event' && isThinking(item.event);
}

/**
 * The items a conversation shows: a batch of thinking progress only as the
 * last item, where it says the model is at work or was when the attempt
 * stopped, or when keep (a link's, a search result's) names it. Everywhere
 * else the reasoning or the reply that followed says the rest.
 */
export function shownItems(items: readonly Item[], keep?: string): readonly Item[] {
  const last = items.length - 1;
  return items.filter((item, i) => !isThinkingItem(item) || i === last || item.id === keep);
}
