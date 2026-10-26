import { describe, expect, it } from 'vitest';
import { lineCount, preview, PREVIEW_CHARS, PREVIEW_LINES } from './text';

describe('preview', () => {
  it('keeps short text whole', () => {
    expect(preview('a\nb\n')).toEqual({ text: 'a\nb\n', lines: 2, clipped: false });
    expect(preview('')).toEqual({ text: '', lines: 0, clipped: false });
  });

  it('cuts after PREVIEW_LINES lines', () => {
    const text = Array.from({ length: 30 }, (_, i) => `line ${String(i)}`).join('\n');
    const p = preview(text);
    expect(p.text.split('\n')).toHaveLength(PREVIEW_LINES);
    expect(p).toMatchObject({ lines: 30, clipped: true });
  });

  it('does not count a final newline as more text', () => {
    const text = 'x\n'.repeat(PREVIEW_LINES);
    expect(preview(text).clipped).toBe(false);
  });

  it('cuts one long line at PREVIEW_CHARS without splitting a surrogate pair', () => {
    expect(preview('a'.repeat(PREVIEW_CHARS * 2))).toMatchObject({ lines: 1, clipped: true });
    const p = preview('a'.repeat(PREVIEW_CHARS - 1) + '😀'.repeat(10));
    expect(p.text.length).toBe(PREVIEW_CHARS - 1);
  });

  it('counts lines', () => {
    expect(lineCount('a')).toBe(1);
    expect(lineCount('a\n')).toBe(1);
    expect(lineCount('a\n\nb')).toBe(3);
  });
});
