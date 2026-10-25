import { describe, expect, it } from 'vitest';
import { unreachable } from './unreachable';

describe('unreachable', () => {
  it('throws with the value it was given', () => {
    // The cast reaches the runtime guard that exhaustive checks make unreachable at compile time.
    expect(() => unreachable({ page: 'unknown' } as never)).toThrow(
      'unhandled variant: {"page":"unknown"}',
    );
  });
});
