// Fabricated events for tests: each call makes the next line of one attempt.
import type { TranscriptEvent } from '../lib/contract';

export const ATTEMPT_ID = '7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0';
const ORIGIN = 'e3b0c44298fc1c14';

type DistributiveOmit<T, K extends PropertyKey> = T extends unknown ? Omit<T, K> : never;

/** An event without what the factory numbers: its kind, payload and optional truncated list. */
export type EventBody = DistributiveOmit<TranscriptEvent, 'seq' | 'id' | 'attempt' | 'source'>;

/** A function making events of attempt on consecutive lines, from line 1. */
export function eventMaker(attempt = ATTEMPT_ID): (body: EventBody) => TranscriptEvent {
  let line = 0;
  return (body) => {
    line++;
    return {
      ...body,
      seq: line,
      id: `${attempt}:${ORIGIN}:${String(line)}:0`,
      attempt,
      source: { line, part: 0 },
    };
  };
}
