// Decoding a run stream's Server-Sent Events (contract/README.md, "SSE
// stream and replay"). The server encodes every update from the contract's
// own types; the UI checks only what it relies on to apply it in order.
import {
  API_PREFIX,
  RESET_REASONS,
  SSE_EVENTS,
  type Live,
  type Reset,
  type RunKey,
} from './contract';
import type { RunChange } from './runstore';

/** A decoded stream message: a run change, or one of the stream's own markers. */
export type StreamMessage =
  | RunChange
  | { readonly type: 'reset'; readonly reset: Reset }
  | { readonly type: 'live'; readonly live: Live };

/** The event names a run stream sends, each listened to. */
export const STREAM_EVENT_NAMES: readonly string[] = Object.values(SSE_EVENTS);

/** The API path of a run's stream, resuming after cursor (`{epoch}:{seq}`). */
export function eventsPath(key: RunKey, cursor: string): string {
  const query = new URLSearchParams({ after: cursor });
  return `${API_PREFIX}/runs/${key.cluster}/${key.namespace}/${key.taskUID}/events?${query.toString()}`;
}

/** Decodes the data of an SSE event named name, or throws on a malformed one. */
export function decodeMessage(name: string, data: string): StreamMessage {
  const value: unknown = JSON.parse(data);
  if (typeof value !== 'object' || value === null) throw new Error(`${name}: not an object`);
  switch (name) {
    case SSE_EVENTS.transcript:
      requireSeq(name, value);
      requireField(name, value, 'id');
      // Checked: the fields the store orders and groups by; the server validated the rest.
      return {
        type: 'transcript',
        event: value as RunChangeOf<'transcript'>['event'],
        size: data.length,
      };
    case SSE_EVENTS.attempt:
      requireSeq(name, value);
      requireField(name, value, 'attempt');
      return { type: 'attempt', update: value as RunChangeOf<'attempt'>['update'] };
    case SSE_EVENTS.run:
      requireSeq(name, value);
      requireField(name, value, 'run');
      return { type: 'run', update: value as RunChangeOf<'run'>['update'] };
    case SSE_EVENTS.end:
      requireSeq(name, value);
      return { type: 'end', end: value as RunChangeOf<'end'>['end'] };
    case SSE_EVENTS.reset:
      if (!('reason' in value) || !isResetReason(value.reason)) {
        throw new Error(`${name}: no reset reason`);
      }
      return { type: 'reset', reset: value as Reset };
    case SSE_EVENTS.live:
      requireSeq(name, value, 'lastSeq');
      return { type: 'live', live: value as Live };
    default:
      throw new Error(`unknown stream event ${name}`);
  }
}

type RunChangeOf<T extends RunChange['type']> = Extract<RunChange, { type: T }>;

function requireSeq(name: string, value: object, field = 'seq'): void {
  const seq: unknown = (value as Record<string, unknown>)[field];
  if (typeof seq !== 'number' || !Number.isSafeInteger(seq) || seq < 0) {
    throw new Error(`${name}: no ${field}`);
  }
}

function requireField(name: string, value: object, field: string): void {
  if (!(field in value)) throw new Error(`${name}: no ${field}`);
}

function isResetReason(reason: unknown): boolean {
  return (RESET_REASONS as readonly unknown[]).includes(reason);
}
