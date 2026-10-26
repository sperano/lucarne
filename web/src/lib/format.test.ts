import { describe, expect, it } from 'vitest';
import { formatBytes, formatCount, formatDuration, formatTime, utf8Length } from './format';

describe('formats', () => {
  it('formats counts, durations, sizes and times', () => {
    expect(formatCount(48384)).toBe('48,384');
    expect(formatDuration(850)).toBe('850 ms');
    expect(formatDuration(48210)).toBe('48.2 s');
    expect(formatDuration(185000)).toBe('3 min 05 s');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(8192)).toBe('8 KiB');
    expect(formatBytes(1536 * 1024)).toBe('1.5 MiB');
    expect(formatTime('2026-10-25T09:00:02Z')).toBe('2026-10-25 09:00:02 UTC');
  });

  it('counts UTF-8 bytes', () => {
    expect(utf8Length('é')).toBe(2);
    expect(utf8Length('')).toBe(0);
  });
});
