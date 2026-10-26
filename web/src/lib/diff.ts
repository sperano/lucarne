// The lines of a unified diff as an edit carries it, classified for
// colouring. The text of each line is kept as written.

export type DiffLineKind = 'add' | 'delete' | 'hunk' | 'header' | 'context';

export interface DiffLine {
  readonly kind: DiffLineKind;
  readonly text: string;
}

const FILE_HEADERS = ['diff ', 'index ', '--- ', '+++ ', 'new file', 'deleted file'];

/** Classifies the lines of diff; file headers count only before the first hunk. */
export function diffLines(diff: string): DiffLine[] {
  const lines = diff.endsWith('\n') ? diff.slice(0, -1).split('\n') : diff.split('\n');
  let inHunk = false;
  return lines.map((text) => {
    if (text.startsWith('@@')) {
      inHunk = true;
      return { kind: 'hunk', text };
    }
    if (!inHunk && FILE_HEADERS.some((h) => text.startsWith(h))) return { kind: 'header', text };
    if (text.startsWith('+')) return { kind: 'add', text };
    if (text.startsWith('-')) return { kind: 'delete', text };
    return { kind: 'context', text };
  });
}
