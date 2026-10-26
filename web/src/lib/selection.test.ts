import { describe, expect, it } from 'vitest';
import type { Attempt } from './contract';
import { attemptOfEvent, selectAttempt } from './selection';

const attempt = (id: string): Attempt => ({
  id,
  key: { podUID: id.split('.')[0] ?? '', container: 'codex', restart: 0 },
  podName: 'example-codex-43-abcde',
  runner: { kind: 'codex' },
  container: { state: 'terminated' },
  coverage: { state: 'ended', lines: 1, gaps: 0, truncatedLines: 0 },
});

const FIRST = attempt('0a1b2c3d-4e5f-4a6b-8c7d-8e9f0a1b2c3d.codex.0');
const LATEST = attempt('9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b.codex.0');
const ATTEMPTS = [FIRST, LATEST];

describe('selectAttempt', () => {
  it('follows the latest attempt by default', () => {
    expect(selectAttempt(ATTEMPTS, undefined, undefined)).toEqual({ attempt: LATEST });
    expect(selectAttempt([], undefined, undefined)).toEqual({});
  });

  it('takes the linked attempt, else the attempt of the linked event', () => {
    expect(selectAttempt(ATTEMPTS, FIRST.id, undefined)).toEqual({ attempt: FIRST });
    expect(selectAttempt(ATTEMPTS, undefined, `${FIRST.id}:e3b0c44298fc1c14:3:0`)).toEqual({
      attempt: FIRST,
    });
  });

  it('says a linked attempt is missing rather than showing another', () => {
    const gone = '1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d.codex.1';
    expect(selectAttempt(ATTEMPTS, gone, undefined)).toEqual({ missing: gone });
  });

  it('reads the attempt of an event ID', () => {
    expect(attemptOfEvent(`${FIRST.id}:e3b0c44298fc1c14:120:gap.1`)).toBe(FIRST.id);
  });
});
