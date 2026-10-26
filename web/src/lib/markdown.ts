// Block Markdown: the subset agents write (paragraphs, ATX headings, fenced
// code, block quotes, lists, GFM tables, rules), parsed into a tree the
// components render as text nodes. Raw HTML is not a block: it stays
// literal text, as everything in a log does.
//
// One pass over the lines per nesting level, and nesting stops at
// MAX_BLOCK_DEPTH, so the work is linear in the text.
import { parseInline, type Inline } from './inline';

/** How deep quotes and lists nest before their markers stay literal. */
export const MAX_BLOCK_DEPTH = 4;

const MAX_HEADING_LEVEL = 6;
const MAX_MARKER_INDENT = 3;
const MIN_FENCE = 3;
const MIN_RULE = 3;
// CommonMark's bound, which keeps the start number exact.
const MAX_ORDINAL_DIGITS = 9;

export type Block =
  | { readonly type: 'paragraph'; readonly children: readonly Inline[] }
  | { readonly type: 'heading'; readonly level: number; readonly children: readonly Inline[] }
  | { readonly type: 'code'; readonly info: string; readonly text: string }
  | { readonly type: 'quote'; readonly children: readonly Block[] }
  | {
      readonly type: 'list';
      readonly ordered: boolean;
      readonly start: number;
      readonly items: readonly (readonly Block[])[];
    }
  | {
      readonly type: 'table';
      readonly header: readonly (readonly Inline[])[];
      readonly rows: readonly (readonly (readonly Inline[])[])[];
    }
  | { readonly type: 'rule' };

interface Parsed {
  readonly block: Block;
  readonly next: number;
}

type BlockParser = (lines: readonly string[], i: number, depth: number) => Parsed | null;

/** Parses Markdown text into blocks. */
export function parseMarkdown(text: string): Block[] {
  return parseBlocks(text.split('\n'), 0);
}

const PARSERS: readonly BlockParser[] = [fence, heading, rule, quote, list, table];

function parseBlocks(lines: readonly string[], depth: number): Block[] {
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    if (isBlank(lines[i])) {
      i++;
      continue;
    }
    const parsed = startBlock(lines, i, depth) ?? paragraph(lines, i);
    blocks.push(parsed.block);
    i = parsed.next;
  }
  return blocks;
}

function startBlock(lines: readonly string[], i: number, depth: number): Parsed | null {
  for (const parser of PARSERS) {
    const parsed = parser(lines, i, depth);
    if (parsed) return parsed;
  }
  return null;
}

function isBlank(line: string | undefined): boolean {
  return line === undefined || line.trim() === '';
}

/** The number of leading spaces of line, tabs counting as one. */
function indentOf(line: string): number {
  let n = 0;
  while (line[n] === ' ' || line[n] === '\t') n++;
  return n;
}

function paragraph(lines: readonly string[], i: number): Parsed {
  let next = i + 1;
  while (next < lines.length && !isBlank(lines[next]) && !interrupts(lines, next)) next++;
  const text = lines
    .slice(i, next)
    .map((l) => l.trim())
    .join('\n');
  return { block: { type: 'paragraph', children: parseInline(text) }, next };
}

/** Whether line i starts a block that ends the paragraph before it. */
function interrupts(lines: readonly string[], i: number): boolean {
  const line = lines[i] ?? '';
  return (
    fenceOpening(line) !== null ||
    headingOf(line) !== null ||
    isRule(line) ||
    quoteContent(line) !== null ||
    listMarker(line) !== null
  );
}

function fenceOpening(line: string): { marker: string; info: string } | null {
  if (indentOf(line) > MAX_MARKER_INDENT) return null;
  const rest = line.trimStart();
  const ch = rest[0];
  if (ch !== '`' && ch !== '~') return null;
  let n = 0;
  while (rest[n] === ch) n++;
  if (n < MIN_FENCE) return null;
  const info = rest.slice(n).trim();
  if (ch === '`' && info.includes('`')) return null;
  return { marker: ch.repeat(n), info: info.split(/\s/, 1)[0] ?? '' };
}

function fence(lines: readonly string[], i: number): Parsed | null {
  const open = fenceOpening(lines[i] ?? '');
  if (!open) return null;
  let end = i + 1;
  while (end < lines.length && !closesFence(lines[end] ?? '', open.marker)) end++;
  const text = lines.slice(i + 1, end).join('\n');
  // An unclosed fence runs to the end of the text, as in CommonMark.
  return { block: { type: 'code', info: open.info, text }, next: end + 1 };
}

function closesFence(line: string, marker: string): boolean {
  const rest = line.trim();
  return indentOf(line) <= MAX_MARKER_INDENT && rest.startsWith(marker) && /^(`+|~+)$/.test(rest);
}

function headingOf(line: string): { level: number; text: string } | null {
  if (indentOf(line) > MAX_MARKER_INDENT) return null;
  const rest = line.trimStart();
  let level = 0;
  while (rest[level] === '#') level++;
  if (level === 0 || level > MAX_HEADING_LEVEL) return null;
  const after = rest[level];
  if (after !== undefined && after !== ' ' && after !== '\t') return null;
  return { level, text: stripClosingHashes(rest.slice(level).trim()) };
}

function stripClosingHashes(text: string): string {
  let end = text.length;
  while (end > 0 && text[end - 1] === '#') end--;
  if (end === 0) return '';
  return end < text.length && (text[end - 1] === ' ' || text[end - 1] === '\t')
    ? text.slice(0, end).trimEnd()
    : text;
}

function heading(lines: readonly string[], i: number): Parsed | null {
  const h = headingOf(lines[i] ?? '');
  if (!h) return null;
  return {
    block: { type: 'heading', level: h.level, children: parseInline(h.text) },
    next: i + 1,
  };
}

function isRule(line: string): boolean {
  if (indentOf(line) > MAX_MARKER_INDENT) return false;
  const marks = line.replace(/[ \t]/g, '');
  const ch = marks[0];
  if ((ch !== '-' && ch !== '*' && ch !== '_') || marks.length < MIN_RULE) return false;
  for (const c of marks) if (c !== ch) return false;
  return true;
}

function rule(lines: readonly string[], i: number): Parsed | null {
  return isRule(lines[i] ?? '') ? { block: { type: 'rule' }, next: i + 1 } : null;
}

function quoteContent(line: string): string | null {
  if (indentOf(line) > MAX_MARKER_INDENT) return null;
  const rest = line.trimStart();
  if (!rest.startsWith('>')) return null;
  return rest.startsWith('> ') ? rest.slice(2) : rest.slice(1);
}

function quote(lines: readonly string[], i: number, depth: number): Parsed | null {
  if (depth >= MAX_BLOCK_DEPTH) return null;
  const inner: string[] = [];
  let next = i;
  for (; next < lines.length; next++) {
    const content = quoteContent(lines[next] ?? '');
    if (content === null) break;
    inner.push(content);
  }
  if (inner.length === 0) return null;
  return { block: { type: 'quote', children: parseBlocks(inner, depth + 1) }, next };
}

interface Marker {
  readonly ordered: boolean;
  readonly start: number;
  // Where the item's content starts: continuation lines indented this far belong to it.
  readonly contentIndent: number;
  readonly content: string;
}

function listMarker(line: string): Marker | null {
  const indent = indentOf(line);
  if (indent > MAX_MARKER_INDENT) return null;
  const rest = line.slice(indent);
  let width = 0;
  while (/[0-9]/.test(rest.charAt(width))) width++;
  const ordered = width > 0;
  if (ordered && (width > MAX_ORDINAL_DIGITS || (rest[width] !== '.' && rest[width] !== ')')))
    return null;
  if (!ordered && rest[0] !== '-' && rest[0] !== '*' && rest[0] !== '+') return null;
  const markerEnd = ordered ? width + 1 : 1;
  const after = rest[markerEnd];
  if (after !== undefined && after !== ' ' && after !== '\t') return null;
  return {
    ordered,
    start: ordered ? Number(rest.slice(0, width)) : 1,
    contentIndent: indent + markerEnd + 1,
    content: rest.slice(markerEnd + 1),
  };
}

function list(lines: readonly string[], i: number, depth: number): Parsed | null {
  if (depth >= MAX_BLOCK_DEPTH) return null;
  const first = listMarker(lines[i] ?? '');
  if (!first) return null;
  const items: Block[][] = [];
  let next = i;
  for (;;) {
    const marker = listMarker(lines[next] ?? '');
    if (!marker || marker.ordered !== first.ordered) break;
    const item = listItem(lines, next, marker);
    items.push(parseBlocks(item.lines, depth + 1));
    next = item.next;
  }
  return { block: { type: 'list', ordered: first.ordered, start: first.start, items }, next };
}

/** The lines of the item whose marker is on line i, its continuation lines dedented. */
function listItem(
  lines: readonly string[],
  i: number,
  marker: Marker,
): { lines: string[]; next: number } {
  const item = [marker.content];
  let next = i + 1;
  for (; next < lines.length; next++) {
    const line = lines[next] ?? '';
    if (isBlank(line)) {
      // A blank line continues the item only when an indented line follows it.
      if (indentOf(lines[next + 1] ?? '') < marker.contentIndent || isBlank(lines[next + 1])) break;
      item.push('');
    } else if (indentOf(line) >= marker.contentIndent) {
      item.push(line.slice(marker.contentIndent));
    } else if (!interrupts(lines, next)) {
      item.push(line.trim()); // a lazy continuation of the item's paragraph
    } else {
      break;
    }
  }
  return { lines: item, next };
}

function tableCells(line: string): string[] {
  let row = line.trim();
  if (row.startsWith('|')) row = row.slice(1);
  if (row.endsWith('|') && !row.endsWith('\\|')) row = row.slice(0, -1);
  return row.split(/(?<!\\)\|/).map((cell) => cell.trim().replaceAll('\\|', '|'));
}

function isDelimiterRow(line: string): boolean {
  if (!line.includes('-')) return false;
  return tableCells(line).every((cell) => /^:?-+:?$/.test(cell));
}

function table(lines: readonly string[], i: number): Parsed | null {
  const head = lines[i] ?? '';
  if (!head.includes('|') || !isDelimiterRow(lines[i + 1] ?? '')) return null;
  const rows: (readonly Inline[])[][] = [];
  let next = i + 2;
  for (; next < lines.length; next++) {
    const line = lines[next] ?? '';
    if (isBlank(line) || !line.includes('|')) break;
    rows.push(tableCells(line).map((cell) => parseInline(cell)));
  }
  const header = tableCells(head).map((cell) => parseInline(cell));
  return { block: { type: 'table', header, rows }, next };
}
