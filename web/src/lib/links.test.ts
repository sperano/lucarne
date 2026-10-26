import { describe, expect, it } from 'vitest';
import { safeHref } from './links';

describe('safeHref', () => {
  it.each([
    ['https://github.com/example/widget/pull/7', 'https://github.com/example/widget/pull/7'],
    ['http://example.com', 'http://example.com/'],
    ['HTTPS://EXAMPLE.COM/a b', 'https://example.com/a%20b'],
    ['mailto:agent@example.com', 'mailto:agent@example.com'],
    ['  https://example.com/x', 'https://example.com/x'],
  ])('keeps %s', (raw, href) => {
    expect(safeHref(raw)).toBe(href);
  });

  it.each([
    'javascript:alert(1)',
    'JavaScript:alert(1)',
    ' javascript:alert(1)',
    'java\tscript:alert(1)',
    'java\nscript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    'file:///etc/passwd',
    'blob:https://example.com/1',
    '/api/v1/contract',
    '//example.com/x',
    'example.com',
    '#event=x',
    '',
  ])('refuses %j', (raw) => {
    expect(safeHref(raw)).toBeNull();
  });
});
