import { describe, expect, it } from 'vitest';
import { MAX_BLOCK_DEPTH, parseMarkdown, type Block } from './markdown';

const text = (t: string) => ({ type: 'text', text: t }) as const;

function depthOf(blocks: readonly Block[]): number {
  return Math.max(
    0,
    ...blocks.map((b) => {
      if (b.type === 'quote') return 1 + depthOf(b.children);
      if (b.type === 'list') return 1 + Math.max(0, ...b.items.map(depthOf));
      return 0;
    }),
  );
}

describe('parseMarkdown', () => {
  it('splits paragraphs on blank lines and keeps their line breaks', () => {
    expect(parseMarkdown('one\ntwo\n\nthree')).toEqual([
      { type: 'paragraph', children: [text('one\ntwo')] },
      { type: 'paragraph', children: [text('three')] },
    ]);
  });

  it('parses ATX headings and strips closing hashes', () => {
    expect(parseMarkdown('# Title\n### Sub ##\n#nospace\n####### seven')).toEqual([
      { type: 'heading', level: 1, children: [text('Title')] },
      { type: 'heading', level: 3, children: [text('Sub')] },
      { type: 'paragraph', children: [text('#nospace\n####### seven')] },
    ]);
  });

  it('parses fenced code verbatim, markup and HTML included', () => {
    const code = '<script>alert(1)</script>\n**not bold**\n\n  indented';
    expect(parseMarkdown('```go extra\n' + code + '\n```\nafter')).toEqual([
      { type: 'code', info: 'go', text: code },
      { type: 'paragraph', children: [text('after')] },
    ]);
    expect(parseMarkdown('~~~\nunclosed\n```')).toEqual([
      { type: 'code', info: '', text: 'unclosed\n```' },
    ]);
  });

  it('parses lists, nested and ordered, interrupting a paragraph', () => {
    expect(parseMarkdown('Summary:\n- one\n  - nested\n- two\n\n3. three\n4. four')).toEqual([
      { type: 'paragraph', children: [text('Summary:')] },
      {
        type: 'list',
        ordered: false,
        start: 1,
        items: [
          [
            { type: 'paragraph', children: [text('one')] },
            {
              type: 'list',
              ordered: false,
              start: 1,
              items: [[{ type: 'paragraph', children: [text('nested')] }]],
            },
          ],
          [{ type: 'paragraph', children: [text('two')] }],
        ],
      },
      {
        type: 'list',
        ordered: true,
        start: 3,
        items: [
          [{ type: 'paragraph', children: [text('three')] }],
          [{ type: 'paragraph', children: [text('four')] }],
        ],
      },
    ]);
  });

  it('parses block quotes and rules', () => {
    expect(parseMarkdown('> quoted\n> more\n\n---')).toEqual([
      { type: 'quote', children: [{ type: 'paragraph', children: [text('quoted\nmore')] }] },
      { type: 'rule' },
    ]);
  });

  it('parses GFM tables, escaped pipes included', () => {
    expect(parseMarkdown('| File | Change |\n| --- | :-: |\n| `a.go` | a \\| b |')).toEqual([
      {
        type: 'table',
        header: [[text('File')], [text('Change')]],
        rows: [[[{ type: 'code', text: 'a.go' }], [text('a | b')]]],
      },
    ]);
  });

  it('keeps raw HTML as paragraph text', () => {
    const html = '<div onclick="alert(1)">\n<iframe src="/x"></iframe>';
    expect(parseMarkdown(html)).toEqual([{ type: 'paragraph', children: [text(html)] }]);
  });

  it('stops nesting at MAX_BLOCK_DEPTH', () => {
    const blocks = parseMarkdown('> '.repeat(MAX_BLOCK_DEPTH + 3) + 'deep');
    expect(depthOf(blocks)).toBe(MAX_BLOCK_DEPTH);
    expect(parseMarkdown('- '.repeat(MAX_BLOCK_DEPTH + 3) + 'deep').length).toBe(1);
  });

  const SIZE = 65536;
  const BUDGET_MS = 1000;
  it.each([
    ['nested quotes', '> '.repeat(SIZE / 2) + 'x'],
    ['nested list markers', '- '.repeat(SIZE / 2) + 'x'],
    ['quote lines', '> a\n'.repeat(SIZE / 4)],
    ['list lines', '- a\n  b\n'.repeat(SIZE / 8)],
    ['table rows', '| a | b |\n|---|---|\n' + '| a | b |\n'.repeat(SIZE / 10)],
    ['fences', '```\n'.repeat(SIZE / 4)],
    ['headings', '#'.repeat(SIZE)],
    ['heading spaces', '# ' + ' '.repeat(SIZE) + '#'],
    ['blank lines', '\n'.repeat(SIZE)],
    ['emphasis in lines', '*a \n'.repeat(SIZE / 4)],
  ])('parses 64 KiB of %s quickly', (_name, input) => {
    const started = performance.now();
    parseMarkdown(input);
    expect(performance.now() - started).toBeLessThan(BUDGET_MS);
  });
});
