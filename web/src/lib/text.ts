// Previews of long text: a block shows its first lines until expanded, so a
// 32 KiB tool output costs a few lines of layout until someone asks for it.

/** Lines a collapsed block shows. */
export const PREVIEW_LINES = 12;
/** Characters a collapsed block shows, for output with few, long lines. */
export const PREVIEW_CHARS = 2000;

export interface Preview {
  readonly text: string;
  /** The lines text holds in full. */
  readonly lines: number;
  /** Whether text is a cut of the full text. */
  readonly clipped: boolean;
}

/** The first maxLines lines of text, at most maxChars characters of them. */
export function preview(text: string, maxLines = PREVIEW_LINES, maxChars = PREVIEW_CHARS): Preview {
  let newline = -1;
  for (let seen = 0; seen < maxLines; seen++) {
    newline = text.indexOf('\n', newline + 1);
    if (newline === -1) break;
  }
  let cut = Math.min(newline === -1 ? text.length : newline, maxChars);
  if (isHighSurrogate(text.charCodeAt(cut - 1))) cut--;
  // A final newline is not more text.
  const rest = text.length - cut;
  const clipped = rest > 1 || (rest === 1 && !text.endsWith('\n'));
  return { text: text.slice(0, cut), lines: lineCount(text), clipped };
}

const HIGH_SURROGATE_FIRST = 0xd800;
const HIGH_SURROGATE_LAST = 0xdbff;

function isHighSurrogate(code: number): boolean {
  return code >= HIGH_SURROGATE_FIRST && code <= HIGH_SURROGATE_LAST;
}

/** The number of lines of text, a trailing newline not starting another. */
export function lineCount(text: string): number {
  if (text === '') return 0;
  let n = 1;
  for (let i = text.indexOf('\n'); i !== -1 && i < text.length - 1; i = text.indexOf('\n', i + 1))
    n++;
  return n;
}
