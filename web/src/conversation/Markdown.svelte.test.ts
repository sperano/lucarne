import { describe, expect, it } from 'vitest';
import { parseMarkdown } from '../lib/markdown';
import { render } from '../render';
import Markdown from './Markdown.svelte';

function show(text: string): HTMLElement {
  return render(Markdown, { blocks: parseMarkdown(text) });
}

// What a hostile or careless log line might hold.
const HOSTILE = [
  '<script>window.__pwned = true</script>',
  '<img src=x onerror="window.__pwned = true">',
  '<iframe src="https://example.com"></iframe>',
  '<a href="javascript:window.__pwned = true">html link</a>',
  '[md link](javascript:window.__pwned=true)',
  '[data link](data:text/html,hi)',
  '<javascript:window.__pwned=true>',
  '![image](https://example.com/tracker.png)',
  '```html\n<script>window.__pwned = true</script>\n```',
  '| <b>cell</b> | x |\n| --- | --- |\n| <svg onload="window.__pwned=true"> | y |',
].join('\n\n');

declare global {
  interface Window {
    __pwned?: boolean;
  }
}

describe('Markdown', () => {
  it('renders log content as text: no markup, no script, no unsafe link', async () => {
    const root = show(HOSTILE);
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(window.__pwned).toBeUndefined();
    expect(root.querySelector('script, img, iframe, svg, b, object, embed')).toBeNull();
    expect(root.textContent).toContain('<script>window.__pwned = true</script>');
    expect(root.textContent).toContain('<img src=x onerror="window.__pwned = true">');
    const hrefs = [...root.querySelectorAll('a')].map((a) => a.getAttribute('href'));
    expect(hrefs.every((h) => h !== null && /^https:\/\//.test(h))).toBe(true);
  });

  it('renders the structure agents write', () => {
    const root = show(
      '## Summary\n\nFixed **two** tests in `greet.go`:\n\n1. one\n2. two\n\n> note\n\n```go\nfunc main() {}\n```\n\nSee https://github.com/example/widget/pull/7.',
    );
    expect(root.querySelector('h4')?.textContent).toBe('Summary');
    expect(root.querySelector('strong')?.textContent).toBe('two');
    expect(root.querySelector('p code')?.textContent).toBe('greet.go');
    expect(root.querySelectorAll('ol li')).toHaveLength(2);
    expect(root.querySelector('blockquote')?.textContent.trim()).toBe('note');
    expect(root.querySelector('pre code')?.textContent).toBe('func main() {}');
    expect(root.querySelector('figcaption')?.textContent).toBe('go');
    const link = root.querySelector('a');
    expect(link?.textContent).toBe('https://github.com/example/widget/pull/7');
    expect(link?.target).toBe('_blank');
    expect(link?.rel).toBe('noopener noreferrer nofollow');
  });

  it('keeps the spaces between inline nodes', () => {
    expect(show('a **b** `c` *d* e').querySelector('p')?.textContent).toBe('a b c d e');
  });
});
