// WCAG 2.2 contrast of the palette in theme.css: 4.5:1 for text (AA), 3:1
// for the focus ring against what it is drawn on (non-text contrast).
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const TEXT_CONTRAST = 4.5;
const NON_TEXT_CONTRAST = 3;

const css = readFileSync(new URL('./theme.css', import.meta.url), 'utf8');
const palette = new Map(
  [...css.matchAll(/--([a-z-]+):\s*(#[0-9a-f]{6});/g)].map((m) => [m[1] ?? '', m[2] ?? '']),
);

// Every text colour on every background it is used on.
const TEXT_PAIRS: readonly (readonly [string, string])[] = [
  ['text', 'bg'],
  ['text', 'surface'],
  ['text', 'user-bg'],
  ['text', 'code-bg'],
  ['text-muted', 'bg'],
  ['text-muted', 'surface'],
  ['text-muted', 'user-bg'],
  ['link', 'bg'],
  ['link', 'surface'],
  ['link', 'user-bg'],
  ['ok', 'bg'],
  ['error', 'bg'],
  ['error-text', 'error-bg'],
  ['warn-text', 'warn-bg'],
  ['info-text', 'info-bg'],
  ['link', 'info-bg'],
  ['diff-add-text', 'diff-add-bg'],
  ['diff-del-text', 'diff-del-bg'],
  ['diff-hunk-text', 'diff-hunk-bg'],
];

const FOCUS_PAIRS: readonly (readonly [string, string])[] = [
  ['focus', 'bg'],
  ['focus', 'surface'],
  ['focus', 'user-bg'],
];

const SRGB_THRESHOLD = 0.04045;
const SRGB_DIVISOR = 12.92;
const SRGB_OFFSET = 0.055;
const SRGB_SCALE = 1.055;
const SRGB_GAMMA = 2.4;
const CHANNEL_MAX = 255;
const LUMINANCE_WEIGHTS = [0.2126, 0.7152, 0.0722];
const FLARE = 0.05;

function luminance(hex: string): number {
  const channels = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / CHANNEL_MAX);
  const linear = channels.map((c) =>
    c <= SRGB_THRESHOLD ? c / SRGB_DIVISOR : ((c + SRGB_OFFSET) / SRGB_SCALE) ** SRGB_GAMMA,
  );
  return linear.reduce((sum, c, i) => sum + c * (LUMINANCE_WEIGHTS[i] ?? 0), 0);
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return ((hi ?? 0) + FLARE) / ((lo ?? 0) + FLARE);
}

function colour(name: string): string {
  const value = palette.get(name);
  if (value === undefined) throw new Error(`theme.css has no --${name}`);
  return value;
}

describe('theme contrast', () => {
  it.each(TEXT_PAIRS)('--%s on --%s meets AA for text', (fg, bg) => {
    expect(contrast(colour(fg), colour(bg))).toBeGreaterThanOrEqual(TEXT_CONTRAST);
  });

  it.each(FOCUS_PAIRS)('--%s on --%s meets non-text contrast', (fg, bg) => {
    expect(contrast(colour(fg), colour(bg))).toBeGreaterThanOrEqual(NON_TEXT_CONTRAST);
  });

  it('computes the reference ratio', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21);
  });
});
