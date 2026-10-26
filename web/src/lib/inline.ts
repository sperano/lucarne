// Inline Markdown: code spans, emphasis, strikethrough, links and bare
// URLs, parsed into a tree the components render as text nodes. Nothing
// here produces HTML, so markup in log content stays literal text.
//
// The parser is linear in its input apart from the nesting it allows
// (MAX_INLINE_DEPTH): every search for a closing delimiter goes through a
// Finder, which never scans the same stretch twice for the same character,
// so 64 KiB of unmatched delimiters cannot stall the page.
import { safeHref } from './links';

/** How deep emphasis and link text nest before delimiters stay literal. */
export const MAX_INLINE_DEPTH = 6;

export type Inline =
  | { readonly type: 'text'; readonly text: string }
  | { readonly type: 'code'; readonly text: string }
  | { readonly type: 'strong'; readonly children: readonly Inline[] }
  | { readonly type: 'em'; readonly children: readonly Inline[] }
  | { readonly type: 'del'; readonly children: readonly Inline[] }
  | { readonly type: 'link'; readonly href: string; readonly children: readonly Inline[] };

type Wrapper = 'strong' | 'em' | 'del';

const BARE_URL_PREFIXES = ['https://', 'http://'];
// Characters that end a bare URL, and trailing ones it does not keep.
const URL_STOP = /[\s<>"'`]/;
const URL_TRAILING = '.,:;!?)]}';
const ESCAPABLE = /[!-/:-@[-`{-~]/;
const WORD = /[\p{L}\p{N}]/u;
const SPACE = /\s/;

/** Memoized forward search for single characters, as the parser scans left to right. */
class Finder {
  private readonly last = new Map<string, { from: number; at: number }>();
  constructor(private readonly text: string) {}

  /** The first index >= from of ch, or -1. */
  find(ch: string, from: number): number {
    const seen = this.last.get(ch);
    if (seen && seen.from <= from && (seen.at === -1 || seen.at >= from)) return seen.at;
    const at = this.text.indexOf(ch, from);
    this.last.set(ch, { from, at });
    return at;
  }
}

interface Match {
  readonly node: Inline;
  readonly end: number;
}

/** Parses inline Markdown. */
export function parseInline(text: string, depth = 0): Inline[] {
  return new InlineParser(text, depth).parse();
}

class InlineParser {
  private readonly finder: Finder;
  private readonly nodes: Inline[] = [];
  private plain = '';
  // Delimiters with no closer left: every later opener of them is literal.
  private readonly exhausted = new Set<string>();

  constructor(
    private readonly text: string,
    private readonly depth: number,
  ) {
    this.finder = new Finder(text);
  }

  parse(): Inline[] {
    let i = 0;
    while (i < this.text.length) {
      const match = this.matchAt(i);
      if (match) {
        this.flush();
        this.nodes.push(match.node);
        i = match.end;
      } else if (this.text[i] === '\\' && ESCAPABLE.test(this.text[i + 1] ?? '')) {
        this.plain += this.text.charAt(i + 1);
        i += 2;
      } else {
        this.plain += this.text.charAt(i);
        i += 1;
      }
    }
    this.flush();
    return this.nodes;
  }

  private flush(): void {
    if (this.plain !== '') this.nodes.push({ type: 'text', text: this.plain });
    this.plain = '';
  }

  private matchAt(i: number): Match | null {
    switch (this.text[i]) {
      case '`':
        return this.codeSpan(i);
      case '*':
      case '_':
        return this.depth < MAX_INLINE_DEPTH ? this.emphasis(i) : null;
      case '~':
        return this.depth < MAX_INLINE_DEPTH ? this.wrapped(i, '~~', 'del') : null;
      case '[':
        return this.depth < MAX_INLINE_DEPTH ? this.link(i) : null;
      case '<':
        return this.autolink(i);
      case 'h':
        return this.bareURL(i);
      default:
        return null;
    }
  }

  private runLength(i: number): number {
    const ch = this.text[i];
    let end = i;
    while (this.text[end] === ch) end++;
    return end - i;
  }

  private codeSpan(i: number): Match | null {
    const fence = this.runLength(i);
    const key = '`'.repeat(fence);
    // An unmatched run of backticks is literal as a whole.
    const literal: Match = { node: { type: 'text', text: key }, end: i + fence };
    if (this.exhausted.has(key)) return literal;
    let from = i + fence;
    for (;;) {
      const at = this.finder.find('`', from);
      if (at === -1) {
        this.exhausted.add(key);
        return literal;
      }
      const run = this.runLength(at);
      if (run === fence) {
        return {
          node: { type: 'code', text: codeSpanText(this.text.slice(i + fence, at)) },
          end: at + run,
        };
      }
      from = at + run;
    }
  }

  private emphasis(i: number): Match | null {
    const ch = this.text[i] ?? '';
    if (this.text[i + 1] === ch) {
      const strong = this.wrapped(i, ch + ch, 'strong');
      if (strong) return strong;
    }
    return this.wrapped(i, ch, 'em');
  }

  /** delim at i opens a node of type when a matching, flanking closer follows. */
  private wrapped(i: number, delim: string, type: Wrapper): Match | null {
    if (this.exhausted.has(delim) || !this.text.startsWith(delim, i)) return null;
    if (!this.opens(i, delim)) return null;
    let from = i + delim.length + 1;
    for (;;) {
      const at = this.finder.find(delim[0] ?? '', from);
      if (at === -1) {
        this.exhausted.add(delim);
        return null;
      }
      if (this.text.startsWith(delim, at) && this.closes(at, delim)) {
        const children = parseInline(this.text.slice(i + delim.length, at), this.depth + 1);
        return { node: { type, children }, end: at + delim.length };
      }
      from = at + 1;
    }
  }

  private opens(i: number, delim: string): boolean {
    const next = this.text[i + delim.length] ?? ' ';
    if (SPACE.test(next) || next === delim[0]) return false;
    return delim[0] !== '_' || !WORD.test(this.text[i - 1] ?? ' ');
  }

  private closes(at: number, delim: string): boolean {
    // A closer is a whole run of its delimiter, after something other than space.
    const prev = this.text[at - 1] ?? ' ';
    if (SPACE.test(prev) || prev === delim[0] || this.text[at + delim.length] === delim[0])
      return false;
    return delim[0] !== '_' || !WORD.test(this.text[at + delim.length] ?? ' ');
  }

  private link(i: number): Match | null {
    const close = this.finder.find(']', i + 1);
    if (close === -1 || this.text[close + 1] !== '(') return null;
    const end = this.finder.find(')', close + 2);
    if (end === -1) return null;
    const href = safeHref(this.text.slice(close + 2, end).trim());
    if (href === null) return null;
    const children = parseInline(this.text.slice(i + 1, close), this.depth + 1);
    return { node: { type: 'link', href, children }, end: end + 1 };
  }

  private autolink(i: number): Match | null {
    const end = this.finder.find('>', i + 1);
    if (end === -1) return null;
    const target = this.text.slice(i + 1, end);
    if (SPACE.test(target)) return null;
    const href = safeHref(target);
    return href === null ? null : { node: linkNode(href, target), end: end + 1 };
  }

  private bareURL(i: number): Match | null {
    if (WORD.test(this.text[i - 1] ?? ' ')) return null;
    if (!BARE_URL_PREFIXES.some((p) => this.text.startsWith(p, i))) return null;
    let end = i;
    while (end < this.text.length && !URL_STOP.test(this.text[end] ?? '')) end++;
    const target = trimURL(this.text.slice(i, end));
    const href = safeHref(target);
    return href === null ? null : { node: linkNode(href, target), end: i + target.length };
  }
}

function linkNode(href: string, text: string): Inline {
  return { type: 'link', href, children: [{ type: 'text', text }] };
}

/**
 * A bare URL without the punctuation that ends the sentence around it; a
 * closing parenthesis stays when the URL opened it, as in a Wikipedia link.
 */
function trimURL(url: string): string {
  const opens = count(url, '(');
  let closes = count(url, ')');
  let end = url.length;
  for (; end > 0; end--) {
    const ch = url.charAt(end - 1);
    if (!URL_TRAILING.includes(ch) || (ch === ')' && closes <= opens)) break;
    if (ch === ')') closes--;
  }
  return url.slice(0, end);
}

function count(text: string, ch: string): number {
  let n = 0;
  for (const c of text) if (c === ch) n++;
  return n;
}

/** CommonMark strips one space on each side of a code span that has both. */
function codeSpanText(inner: string): string {
  const padded = inner.length > 2 && inner.startsWith(' ') && inner.endsWith(' ');
  return padded && inner.trim() !== '' ? inner.slice(1, -1) : inner;
}
