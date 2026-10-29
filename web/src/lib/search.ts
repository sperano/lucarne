// Searching and filtering the conversation of one attempt. Both work on the
// items the page holds, collapsed content included: the events loaded, not
// those the run dropped to stay within its bounds or those not read yet.
import type { TranscriptEvent } from './contract';
import type { Item } from './conversation';
import { unreachable } from './unreachable';

export const ITEM_FILTERS = ['all', 'tools', 'errors'] as const;
export type ItemFilter = (typeof ITEM_FILTERS)[number];

export const FILTER_LABELS: Record<ItemFilter, string> = {
  all: 'Everything',
  tools: 'Tool calls',
  errors: 'Errors',
};

// Items never change (conversation.ts replaces them), so their text is computed once.
const texts = new WeakMap<Item, string>();

/** Whether item passes filter: tool calls (and results or edits shown alone), or failures. */
export function passes(item: Item, filter: ItemFilter): boolean {
  switch (filter) {
    case 'all':
      return true;
    case 'tools':
      return (
        item.kind === 'tool' || ['tool_progress', 'tool_result', 'edit'].includes(item.event.kind)
      );
    case 'errors':
      return failed(item);
    default:
      return unreachable(filter);
  }
}

/** The IDs of the items, in order, whose text contains query, ignoring case. */
export function search(items: readonly Item[], query: string): string[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') return [];
  return items.filter((item) => textOf(item).includes(needle)).map((item) => item.id);
}

/** The text a search looks in, lowercased: what the item shows, expanded. */
export function textOf(item: Item): string {
  let text = texts.get(item);
  if (text === undefined) {
    const parts = item.kind === 'tool' ? toolTexts(item) : eventTexts(item.event);
    text = parts
      .filter((p) => p !== undefined && p !== '')
      .join('\n')
      .toLowerCase();
    texts.set(item, text);
  }
  return text;
}

function failed(item: Item): boolean {
  if (item.kind === 'tool') {
    if (isFailure(item.result?.toolResult.status)) return true;
    const phase = item.progress.at(-1)?.toolProgress.phase;
    return phase === 'failed' || phase === 'killed';
  }
  return failedEvent(item.event);
}

/** Whether event reports a failure: a runner error, failed tool event, or failed phase. */
export function failedEvent(event: TranscriptEvent): boolean {
  switch (event.kind) {
    case 'error':
      return true;
    case 'tool_result':
      return isFailure(event.toolResult.status);
    case 'tool_progress':
      return event.toolProgress.phase === 'failed' || event.toolProgress.phase === 'killed';
    case 'lifecycle':
      return (
        event.lifecycle.isError === true ||
        event.lifecycle.phase === 'setup_failed' ||
        event.lifecycle.phase === 'turn_failed'
      );
    default:
      return false;
  }
}

function isFailure(status: string | undefined): boolean {
  return status === 'error' || status === 'declined';
}

export type Text = string | undefined;

function toolTexts(item: Extract<Item, { kind: 'tool' }>): Text[] {
  const call = item.call.toolCall;
  return [
    call.name,
    call.title,
    call.input === undefined ? call.inputText : JSON.stringify(call.input),
    item.result?.toolResult.output,
    ...item.progress.flatMap((e) => eventTexts(e)),
    ...item.edits.flatMap((e) => eventTexts(e)),
  ];
}

/** The texts an event shows, expanded: what a search looks in. */
export function eventTexts(event: TranscriptEvent): Text[] {
  switch (event.kind) {
    case 'message':
      return [event.message.text];
    case 'tool_call': {
      const call = event.toolCall;
      return [
        call.name,
        call.title,
        call.input === undefined ? call.inputText : JSON.stringify(call.input),
      ];
    }
    case 'tool_result':
      return [event.toolResult.output];
    case 'tool_progress': {
      const progress = event.toolProgress;
      return [progress.phase, progress.detail, progress.subtype];
    }
    case 'edit':
      return [event.edit.path, event.edit.diff, event.edit.before, event.edit.after];
    case 'usage':
      return [];
    case 'error':
      return [event.error.message, event.error.code];
    case 'lifecycle': {
      const l = event.lifecycle;
      return [
        l.phase,
        l.subtype,
        l.stopReason,
        l.detail,
        l.model,
        ...(l.outputs ?? []).map((o) => `${o.key}: ${o.value}`),
      ];
    }
    case 'raw':
      return [event.raw.text];
    case 'gap':
      return [event.gap.reason, event.gap.detail];
    default:
      return unreachable(event);
  }
}
