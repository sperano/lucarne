// What the inspector says of an event beyond its content: the events it is
// tied to (a call, its results and edits, the call that made it), and why a
// call has no result. Only what the attempt's events and coverage show: a
// result that is not in the events held is said to be missing, and why it
// may be, never taken for a success.
import type { Attempt, Stream, TranscriptEvent } from './contract';
import type { Conversation, EventOf } from './conversation';
import { callIdOf } from './filters';
import { formatCount } from './format';

/** Sub-calls listed at most; the rest are counted. */
export const MAX_SUBCALLS = 20;

export type Relation = 'call' | 'revision' | 'progress' | 'result' | 'edit' | 'parent' | 'subcall';

export const RELATION_LABELS: Record<Relation, string> = {
  call: 'Call',
  revision: 'Earlier input',
  progress: 'Progress',
  result: 'Result',
  edit: 'Edit',
  parent: 'Called from',
  subcall: 'Sub-call',
};

export interface Related {
  readonly relation: Relation;
  readonly event: TranscriptEvent;
}

export interface Correlation {
  /** The call ID the event names, if any. */
  readonly callId?: string;
  /** The events tied to it, in the log's order, the event itself included. */
  readonly related: readonly Related[];
  /** Sub-calls past MAX_SUBCALLS, not listed. */
  readonly moreSubcalls: number;
}

/** The events tied to event in its attempt's conversation through its call ID. */
export function correlate(event: TranscriptEvent, conversation: Conversation): Correlation {
  const callId = callIdOf(event);
  if (callId === undefined) return { related: [], moreSubcalls: 0 };
  const related: Related[] = [];
  let subcalls = 0;
  const calls = conversation.events.filter(
    (e): e is EventOf<'tool_call'> => e.kind === 'tool_call' && e.toolCall.callId === callId,
  );
  const parentId = calls.at(-1)?.toolCall.parentCallId;
  for (const e of conversation.events) {
    const relation = relationOf(e, callId, parentId, calls);
    if (relation === undefined) continue;
    if (relation === 'subcall' && ++subcalls > MAX_SUBCALLS) continue;
    related.push({ relation, event: e });
  }
  return { callId, related, moreSubcalls: Math.max(0, subcalls - MAX_SUBCALLS) };
}

function relationOf(
  e: TranscriptEvent,
  callId: string,
  parentId: string | undefined,
  calls: readonly EventOf<'tool_call'>[],
): Relation | undefined {
  switch (e.kind) {
    case 'tool_call':
      if (e.toolCall.callId === callId) return e === calls.at(-1) ? 'call' : 'revision';
      if (parentId !== undefined && e.toolCall.callId === parentId) return 'parent';
      return e.toolCall.parentCallId === callId ? 'subcall' : undefined;
    case 'tool_result':
      return e.toolResult.callId === callId ? 'result' : undefined;
    case 'tool_progress':
      return e.toolProgress.callId === callId ? 'progress' : undefined;
    case 'edit':
      return e.edit.callId === callId ? 'edit' : undefined;
    default:
      return undefined;
  }
}

/**
 * What the attempt's events and coverage say of a call and its result
 * when either is missing or says nothing of success: undefined when both
 * are held and the result has a status, else a sentence saying what is
 * missing and why it may be.
 */
export function callEvidence(
  correlation: Correlation,
  attempt: Attempt,
  stream: Stream,
): string | undefined {
  if (correlation.callId === undefined) return undefined;
  const has = (r: Relation) => correlation.related.some((x) => x.relation === r);
  if (!has('call')) return missingCall(stream);
  if (!has('result')) {
    const progress = correlation.related
      .filter((r) => r.event.kind === 'tool_progress')
      .at(-1)?.event;
    if (progress?.kind === 'tool_progress' && terminalProgress(progress.toolProgress.phase)) {
      return `The runner reported the call ${progress.toolProgress.phase}, but emitted no tool result.`;
    }
    return missingResult(attempt);
  }
  const unknown = correlation.related.every(
    (r) => r.event.kind !== 'tool_result' || r.event.toolResult.status === 'unknown',
  );
  return unknown ? 'The result does not say whether the call succeeded.' : undefined;
}

function terminalProgress(phase: string): boolean {
  return phase === 'completed' || phase === 'failed' || phase === 'killed';
}

function missingCall(stream: Stream): string {
  return stream.droppedEvents > 0
    ? `Its call is not among the events held: it may be among the ${formatCount(stream.droppedEvents)} oldest events the run dropped.`
    : 'Its call is not in the log as read.';
}

function missingResult(attempt: Attempt): string {
  const c = attempt.coverage;
  const reason = c.reason === undefined ? '' : ` (${c.reason})`;
  switch (c.state) {
    case 'pending':
    case 'following':
      return 'No result yet: the log is still being read.';
    case 'unavailable':
      return `No result in what was read: the log cannot be read now${reason}, and the result may be in the part not read.`;
    case 'expired':
      return `No result in what was read: the log is gone${reason}, and the result may have been in the part not read.`;
    case 'ended':
      return c.gaps > 0 || c.truncatedLines > 0
        ? `No result in the log as read: ${lossOf(attempt)}, and the result may be in what is missing.`
        : 'The log was read to its end, with no gap and no line cut, and has no result for this call.';
  }
}

function lossOf(attempt: Attempt): string {
  const { gaps, truncatedLines } = attempt.coverage;
  const parts = [
    gaps > 0 ? `${formatCount(gaps)} ${gaps === 1 ? 'gap' : 'gaps'}` : '',
    truncatedLines > 0
      ? `${formatCount(truncatedLines)} ${truncatedLines === 1 ? 'line' : 'lines'} cut`
      : '',
  ].filter((p) => p !== '');
  return `it has ${parts.join(' and ')}`;
}
