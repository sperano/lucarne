// Runner notices: the lines a runner prints around the conversation (hooks,
// statuses, kinds no adapter knows), each one muted line that the reader
// can hide. A notice that reports something going wrong is not one of them.
import type { RawReason, TranscriptEvent } from './contract';
import type { EventOf, Item } from './conversation';

/** The raw reasons of well-formed lines no adapter rendered: notices, not damage. */
const QUIET_RAW_REASONS = new Set<RawReason>(['unknown_type', 'unsupported']);

/** Whether event is a runner notice the reader may hide. */
export function isRunnerNotice(event: TranscriptEvent): boolean {
  switch (event.kind) {
    case 'lifecycle':
      return event.lifecycle.phase === 'runner_notice' && event.lifecycle.isError !== true;
    case 'raw':
      return QUIET_RAW_REASONS.has(event.raw.reason);
    default:
      return false;
  }
}

/** Whether event is a raw line that is damaged: cut JSON, or a line over the size limit. */
export function isDamaged(event: EventOf<'raw'>): boolean {
  return event.raw.reason === 'malformed_json' || event.raw.reason === 'oversized';
}

/** The items without the runner notices, but for the one keep (a link's, a search result's) names. */
export function withoutNotices(items: readonly Item[], keep?: string): readonly Item[] {
  return items.filter(
    (item) => item.kind !== 'event' || !isRunnerNotice(item.event) || item.id === keep,
  );
}

/** The name of a notice's kind: the line's type and the runner's subtype when they differ. */
export function noticeName(type: string | undefined, subtype: string | undefined): string {
  if (subtype === undefined || subtype === '') return type ?? 'notice';
  if (type === undefined || type === '' || type === subtype) return subtype;
  return `${type} · ${subtype}`;
}

/** A notice detail longer than this, or of more than one line, opens on demand. */
export const INLINE_DETAIL_CHARS = 160;

/** The line of a notice's detail shown inline, and whether the whole detail opens on demand. */
export function detailLine(detail: string | undefined): { first: string; more: boolean } {
  if (detail === undefined) return { first: '', more: false };
  const end = detail.indexOf('\n');
  const first = end === -1 ? detail : detail.slice(0, end);
  return { first, more: end !== -1 || first.length > INLINE_DETAIL_CHARS };
}
