import { describe, expect, it } from 'vitest';
import { LIMITS } from '../dev/fixtures';
import { fieldLimit } from './truncation';

describe('fieldLimit', () => {
  it('maps payload fields to their limits', () => {
    expect(fieldLimit('output', LIMITS)).toBe(LIMITS.maxToolOutputBytes);
    expect(fieldLimit('diff', LIMITS)).toBe(LIMITS.maxDiffBytes);
    expect(fieldLimit('text', LIMITS)).toBe(LIMITS.maxTextBytes);
    expect(fieldLimit('inputText', LIMITS)).toBe(LIMITS.maxToolInputBytes);
    expect(fieldLimit('callId', LIMITS)).toBe(LIMITS.maxLabelBytes);
  });
});
