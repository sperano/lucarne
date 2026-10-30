// The inspector's model: one row per event of an attempt, what the run
// context says of the attempt, and how the keyboard moves through the rows.
// Every value is one the source reported; what it did not report is left
// undefined for the view to call unknown, never derived from something else.
import type {
  CoverageState,
  Lifecycle,
  Media,
  Message,
  ToolResultStatus,
  TranscriptEvent,
  Usage,
} from './contract';
import type { Conversation, EventOf } from './conversation';
import {
  EDIT_LABELS,
  EVENT_KIND_LABELS,
  GAP_REASON_LABELS,
  LIFECYCLE_LABELS,
  RAW_REASON_LABELS,
  STATUS_LABELS,
  TOOL_PROGRESS_LABELS,
  USAGE_SCOPE_LABELS,
} from './labels';
import { mediaSummary } from './media';
import { failedEvent } from './search';
import {
  effectiveToolOutcome,
  isFailedProgressPhase,
  isTerminalProgressPhase,
} from './tool-progress';
import { describeTool } from './tools';
import { unreachable } from './unreachable';
import { usageParts } from './usage';
import { earlier, later, windowAround, WINDOW_MAX, type ItemWindow } from './window';

/** A row's summary is cut to this many characters: the detail pane shows the rest. */
export const SUMMARY_CHARS = 160;
/** How many rows PageUp and PageDown move the selection. */
export const PAGE_ROWS = 20;

const ROLE_LABELS = { user: 'User', assistant: 'Assistant' } as const;
const ELLIPSIS = '…';

/** How a row's status reads: a success, a failure, or neither. */
export type Tone = 'ok' | 'error' | 'muted';

export interface Row {
  readonly kind: string;
  readonly summary: string;
  readonly status?: { readonly label: string; readonly tone: Tone };
  /** Whether the event reports a failure, or is the call of a failed result. */
  readonly failed: boolean;
}

/** The row of event in conversation, the attempt it belongs to. */
export function rowOf(
  event: TranscriptEvent,
  conversation: Conversation,
  coverage: CoverageState,
): Row {
  const kind = EVENT_KIND_LABELS[event.kind];
  if (event.kind === 'tool_call') return callRow(event, conversation, coverage);
  if (event.kind === 'tool_result') {
    const r = event.toolResult;
    const name = conversation.calls.get(r.callId)?.call.toolCall.name ?? `call ${r.callId}`;
    const output = r.output === undefined ? noOutput(r.media) : firstLine(r.output);
    return {
      kind,
      summary: cut(`${name}: ${output}`),
      status: resultStatus(r.status, r.exitCode),
      failed: failedEvent(event),
    };
  }
  return { kind, summary: cut(summaryOf(event)), failed: failedEvent(event) };
}

function callRow(
  event: EventOf<'tool_call'>,
  conversation: Conversation,
  coverage: CoverageState,
): Row {
  const call = event.toolCall;
  const subject = describeTool(call).subject;
  const summary = cut(subject === '' ? call.name : `${call.name} ${subject}`);
  const tool = conversation.calls.get(call.callId);
  const outcome = tool === undefined ? undefined : effectiveToolOutcome(tool);
  if (outcome?.kind === 'tool_progress') {
    const phase = outcome.toolProgress.phase;
    const tone: Tone = isTerminalProgressPhase(phase)
      ? isFailedProgressPhase(phase)
        ? 'error'
        : 'ok'
      : 'muted';
    return {
      kind: EVENT_KIND_LABELS.tool_call,
      summary,
      status: { label: TOOL_PROGRESS_LABELS[phase], tone },
      failed: isFailedProgressPhase(phase),
    };
  }
  if (outcome?.kind === 'tool_result') {
    const result = outcome.toolResult;
    const status = resultStatus(result.status, result.exitCode);
    return { kind: EVENT_KIND_LABELS.tool_call, summary, status, failed: status.tone === 'error' };
  }
  const pending = coverage === 'following' ? 'running' : 'no result';
  return {
    kind: EVENT_KIND_LABELS.tool_call,
    summary,
    status: { label: pending, tone: 'muted' },
    failed: false,
  };
}

function resultStatus(
  status: ToolResultStatus,
  exitCode: number | undefined,
): NonNullable<Row['status']> {
  const exit = exitCode === undefined ? '' : `, exit ${String(exitCode)}`;
  const tone: Tone = status === 'ok' ? 'ok' : status === 'unknown' ? 'muted' : 'error';
  return { label: `${STATUS_LABELS[status]}${exit}`, tone };
}

function summaryOf(event: Exclude<TranscriptEvent, EventOf<'tool_call' | 'tool_result'>>): string {
  switch (event.kind) {
    case 'message': {
      const m = event.message;
      const what =
        m.type === 'reasoning' ? `${ROLE_LABELS[m.role]} reasoning` : ROLE_LABELS[m.role];
      return `${what}: ${messageSummary(m)}`;
    }
    case 'edit':
      return `${EDIT_LABELS[event.edit.change]} ${event.edit.path}`;
    case 'tool_progress': {
      const progress = event.toolProgress;
      const detail = progress.detail === undefined ? '' : `: ${firstLine(progress.detail)}`;
      return `${TOOL_PROGRESS_LABELS[progress.phase]}${detail}`;
    }
    case 'usage': {
      const parts = usageParts(event.usage);
      const scope = USAGE_SCOPE_LABELS[event.usage.scope];
      return parts.length === 0 ? `${scope}: no counter` : `${scope}: ${parts.join(', ')}`;
    }
    case 'error':
      return firstLine(event.error.message);
    case 'lifecycle':
      return lifecycleSummary(event.lifecycle);
    case 'raw':
      return `${RAW_REASON_LABELS[event.raw.reason]}: ${firstLine(event.raw.text)}`;
    case 'gap':
      return GAP_REASON_LABELS[event.gap.reason];
    default:
      return unreachable(event);
  }
}

function lifecycleSummary(l: Lifecycle): string {
  const detail = l.model ?? l.subtype ?? l.stopReason ?? l.detail;
  const label = LIFECYCLE_LABELS[l.phase];
  return detail === undefined ? label : `${label}: ${firstLine(detail)}`;
}

/** A message's first line, then the media that came with it: "Look (1 image)". */
function messageSummary(m: Message): string {
  const media = mediaSummary(m.media ?? []);
  if (media === '') return firstLine(m.text);
  return m.text === '' ? media : `${firstLine(m.text)} (${media})`;
}

/** What a result without output carried instead: its media, or nothing. */
function noOutput(media: readonly Media[] | undefined): string {
  const summary = mediaSummary(media ?? []);
  return summary === '' ? 'no output' : summary;
}

/** The first line of text, at most SUMMARY_CHARS of it: a row never holds a whole output. */
function firstLine(text: string): string {
  const newline = text.indexOf('\n');
  const line = newline === -1 ? text : text.slice(0, newline);
  return line.slice(0, SUMMARY_CHARS + 1).trim();
}

function cut(text: string): string {
  return text.length <= SUMMARY_CHARS ? text : `${text.slice(0, SUMMARY_CHARS - 1)}${ELLIPSIS}`;
}

/** What the attempt's own log says of its session; each absent when no event reported it. */
export interface SessionFacts {
  readonly model?: string;
  readonly sessionId?: string;
  readonly runnerVersion?: string;
  /** The last session-wide usage the runner reported, never a sum of smaller scopes. */
  readonly usage?: Usage;
  /** How many usage events of a smaller scope the log has. */
  readonly partialUsages: number;
}

/** The latest value each lifecycle event and session usage of events reported. */
export function sessionFacts(events: readonly TranscriptEvent[]): SessionFacts {
  let model: string | undefined;
  let sessionId: string | undefined;
  let runnerVersion: string | undefined;
  let usage: Usage | undefined;
  let partialUsages = 0;
  for (const event of events) {
    if (event.kind === 'lifecycle') {
      model = event.lifecycle.model ?? model;
      sessionId = event.lifecycle.sessionId ?? sessionId;
      runnerVersion = event.lifecycle.runnerVersion ?? runnerVersion;
    } else if (event.kind === 'usage') {
      if (event.usage.scope === 'session') usage = event.usage;
      else partialUsages++;
    }
  }
  return {
    ...(model === undefined ? {} : { model }),
    ...(sessionId === undefined ? {} : { sessionId }),
    ...(runnerVersion === undefined ? {} : { runnerVersion }),
    ...(usage === undefined ? {} : { usage }),
    partialUsages,
  };
}

/** The row a key moves the selection to among total rows; undefined for a key that does not move it. */
export function moveSelection(
  key: string,
  index: number | undefined,
  total: number,
): number | undefined {
  if (total === 0) return undefined;
  const last = total - 1;
  const clamp = (i: number) => Math.min(last, Math.max(0, i));
  switch (key) {
    case 'ArrowDown':
      return index === undefined ? 0 : clamp(index + 1);
    case 'ArrowUp':
      return index === undefined ? last : clamp(index - 1);
    case 'PageDown':
      return clamp((index ?? -1) + PAGE_ROWS);
    case 'PageUp':
      return clamp((index ?? total) - PAGE_ROWS);
    case 'Home':
      return 0;
    case 'End':
      return last;
    default:
      return undefined;
  }
}

/** The window that shows row index: w itself, w grown by a step next to it, or one around it. */
export function reveal(w: ItemWindow, index: number, total: number): ItemWindow {
  if (index >= w.start && index < w.end) return w;
  if (index === w.end) return later(w, total);
  if (index === w.start - 1) return earlier(w);
  return windowAround(index, total);
}

/**
 * The window after the rows changed: kept, where the rows shown before are
 * now, grown to the new end when it showed the end before, so a reader at
 * the end sees the events that arrive.
 */
export function keepEnd(kept: ItemWindow, wasAtEnd: boolean, total: number): ItemWindow {
  if (!wasAtEnd) return kept;
  return { start: Math.max(kept.start, total - WINDOW_MAX), end: total };
}
