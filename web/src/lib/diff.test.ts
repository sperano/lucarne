import { describe, expect, it } from 'vitest';
import { diffLines } from './diff';

describe('diffLines', () => {
  it('classifies headers, hunks, additions, deletions and context', () => {
    expect(
      diffLines('--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n ctx\n-old\n+new\n--- not a header\n'),
    ).toEqual([
      { kind: 'header', text: '--- a/x' },
      { kind: 'header', text: '+++ b/x' },
      { kind: 'hunk', text: '@@ -1,2 +1,2 @@' },
      { kind: 'context', text: ' ctx' },
      { kind: 'delete', text: '-old' },
      { kind: 'add', text: '+new' },
      { kind: 'delete', text: '--- not a header' },
    ]);
  });
});
