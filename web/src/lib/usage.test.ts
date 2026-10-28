import { describe, expect, it } from 'vitest';
import { usageParts } from './usage';

describe('usageParts', () => {
  it('formats only the counters the runner reported', () => {
    expect(usageParts({ scope: 'session' })).toEqual([]);
    expect(
      usageParts({
        scope: 'session',
        inputTokens: 48384,
        outputTokens: 912,
        costUSD: '0.4213',
        durationMs: 48210,
        turns: 1,
      }),
    ).toEqual(['48,384 input tokens', '912 output tokens', '$0.4213', '48.2 s', '1 turn']);
    expect(usageParts({ scope: 'turn', turns: 3, apiDurationMs: 850 })).toEqual([
      'API 850 ms',
      '3 turns',
    ]);
  });
});
