// The conversation view's model: one attempt's events in order, a tool call
// grouped with its result and the edits it made, everything else one item
// per event. Nothing is dropped: an event that cannot be grouped (a result
// or an edit whose call is not in the transcript, a second result for one
// call) stays an item of its own.
import type { EventKind, TranscriptEvent } from './contract';

export type EventOf<K extends EventKind> = Extract<TranscriptEvent, { kind: K }>;

export interface ToolItem {
  readonly kind: 'tool';
  /** The ID of the call's first event. */
  readonly id: string;
  /** The latest event of the call: a repeated callId supersedes the input. */
  call: EventOf<'tool_call'>;
  /** How many times a later event replaced the call's input. */
  revisions: number;
  result?: EventOf<'tool_result'>;
  readonly edits: EventOf<'edit'>[];
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
  return builder;
}

class ConversationBuilder implements Conversation {
  readonly items: Item[] = [];
  readonly itemOf = new Map<string, number>();
  readonly calls = new Map<string, ToolItem>();

  add(event: TranscriptEvent): void {
    switch (event.kind) {
      case 'tool_call':
        this.addCall(event);
        return;
      case 'tool_result': {
        const tool = this.calls.get(event.toolResult.callId);
        if (tool && !tool.result) {
          tool.result = event;
          this.showIn(tool, event.id);
        } else this.push({ kind: 'event', id: event.id, event });
        return;
      }
      case 'edit': {
        const callId = event.edit.callId;
        const tool = callId === undefined ? undefined : this.calls.get(callId);
        if (tool) {
          tool.edits.push(event);
          this.showIn(tool, event.id);
        } else this.push({ kind: 'event', id: event.id, event });
        return;
      }
      default:
        this.push({ kind: 'event', id: event.id, event });
    }
  }

  private addCall(event: EventOf<'tool_call'>): void {
    const tool = this.calls.get(event.toolCall.callId);
    if (tool) {
      tool.call = event;
      tool.revisions++;
      this.showIn(tool, event.id);
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

  private push(item: Item): void {
    this.itemOf.set(item.id, this.items.length);
    this.items.push(item);
  }

  /** Shows event id in the item of tool, which was pushed when its call came. */
  private showIn(tool: ToolItem, id: string): void {
    const index = this.itemOf.get(tool.id);
    if (index !== undefined) this.itemOf.set(id, index);
  }
}
