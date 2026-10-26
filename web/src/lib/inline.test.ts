import { describe, expect, it } from 'vitest';
import { MAX_INLINE_DEPTH, parseInline, type Inline } from './inline';

// The literal text a tree renders, as a reader sees it.
function textOf(nodes: readonly Inline[]): string {
  return nodes.map((n) => ('children' in n ? textOf(n.children) : n.text)).join('');
}

function depthOf(nodes: readonly Inline[]): number {
  return Math.max(0, ...nodes.map((n) => ('children' in n ? 1 + depthOf(n.children) : 0)));
}

describe('parseInline', () => {
  it('keeps plain text, HTML included, as text', () => {
    const html = 'see <img src=x onerror=alert(1)> and <script>alert(1)</script>';
    expect(parseInline(html)).toEqual([{ type: 'text', text: html }]);
  });

  it('parses code spans, emphasis and strikethrough', () => {
    expect(parseInline('run `go test` **now**, *please* ~~not~~')).toEqual([
      { type: 'text', text: 'run ' },
      { type: 'code', text: 'go test' },
      { type: 'text', text: ' ' },
      { type: 'strong', children: [{ type: 'text', text: 'now' }] },
      { type: 'text', text: ', ' },
      { type: 'em', children: [{ type: 'text', text: 'please' }] },
      { type: 'text', text: ' ' },
      { type: 'del', children: [{ type: 'text', text: 'not' }] },
    ]);
  });

  it('keeps markup inside code spans literal', () => {
    expect(parseInline('`` a ` **b** `` c')).toEqual([
      { type: 'code', text: 'a ` **b**' },
      { type: 'text', text: ' c' },
    ]);
  });

  it('leaves snake_case, spaced asterisks and unmatched delimiters alone', () => {
    for (const text of ['a_b_c', '2 * 3 * 4', '**open', '`open', 'a ~~ b', '[x](', '<a']) {
      expect(textOf(parseInline(text)), text).toBe(text);
      expect(parseInline(text).every((n) => n.type === 'text')).toBe(true);
    }
  });

  it('honours backslash escapes', () => {
    expect(parseInline('\\*not em\\*')).toEqual([{ type: 'text', text: '*not em*' }]);
  });

  it('links safe URLs only', () => {
    expect(parseInline('[the PR](https://github.com/example/widget/pull/7)')).toEqual([
      {
        type: 'link',
        href: 'https://github.com/example/widget/pull/7',
        children: [{ type: 'text', text: 'the PR' }],
      },
    ]);
    const unsafe = '[click](javascript:alert(1))';
    expect(parseInline(unsafe).some((n) => n.type === 'link')).toBe(false);
    expect(textOf(parseInline(unsafe))).toBe(unsafe);
    expect(parseInline('<javascript:alert(1)>').some((n) => n.type === 'link')).toBe(false);
  });

  it('links autolinks and bare URLs without trailing punctuation', () => {
    expect(parseInline('<https://example.com/a>')).toEqual([
      {
        type: 'link',
        href: 'https://example.com/a',
        children: [{ type: 'text', text: 'https://example.com/a' }],
      },
    ]);
    const nodes = parseInline('See https://example.com/a_(b), then (https://example.com/c).');
    const links = nodes.filter((n) => n.type === 'link').map((n) => n.href);
    expect(links).toEqual(['https://example.com/a_(b)', 'https://example.com/c']);
    expect(textOf(nodes)).toBe('See https://example.com/a_(b), then (https://example.com/c).');
  });

  it('does not link a URL inside a word', () => {
    expect(parseInline('xhttps://example.com').every((n) => n.type === 'text')).toBe(true);
  });

  it('nests distinct delimiters', () => {
    const nodes = parseInline('**a _b ~~c [d *e __f__ e* d](https://example.com) c~~ b_ a**');
    expect(depthOf(nodes)).toBe(MAX_INLINE_DEPTH);
    expect(textOf(nodes)).toBe('a b c d e f e d c b a');
  });

  it('keeps delimiters literal past MAX_INLINE_DEPTH', () => {
    expect(parseInline('**a** [b](https://example.com)', MAX_INLINE_DEPTH)).toEqual([
      { type: 'text', text: '**a** [b](' },
      {
        type: 'link',
        href: 'https://example.com/',
        children: [{ type: 'text', text: 'https://example.com' }],
      },
      { type: 'text', text: ')' },
    ]);
  });

  // Each input would take seconds or minutes in a parser that rescans for
  // closers; at 64 KiB (maxTextBytes) this one must stay well under the budget.
  const SIZE = 65536;
  const BUDGET_MS = 1000;
  it.each([
    ['emphasis openers', '*a '.repeat(SIZE / 3)],
    ['underscores', '_a'.repeat(SIZE / 2)],
    ['asterisks', '*'.repeat(SIZE)],
    [
      'backticks of growing length',
      Array.from({ length: 300 }, (_, i) => '`'.repeat(i + 1) + 'a').join(''),
    ],
    ['backticks', '`'.repeat(SIZE)],
    ['brackets', '['.repeat(SIZE)],
    ['links', '[a]('.repeat(SIZE / 4)],
    ['angle brackets', '<'.repeat(SIZE)],
    ['URLs', 'https://a.'.repeat(SIZE / 10)],
    ['closing parentheses', 'https://a' + ')'.repeat(SIZE)],
    ['mixed', '**_`[<~~'.repeat(SIZE / 8)],
  ])('parses 64 KiB of %s quickly', (_name, text) => {
    const started = performance.now();
    const nodes = parseInline(text);
    expect(performance.now() - started).toBeLessThan(BUDGET_MS);
    expect(textOf(nodes).length).toBeGreaterThan(0);
  });
});
