// Hostile log content is shown as text and never acts: no script runs, no
// unsafe link, no element with a handler, no request off the server, no
// dialog. The document carries its security headers.
import type { Browser, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { baseURL, run } from './control';
import {
  assistantMessages,
  close,
  launch,
  open,
  transcript,
  visible,
  type Watched,
} from './harness';

/** The property the hostile run's scripts set if they ever ran (scenario.HostileGlobal). */
const HOSTILE_GLOBAL = '__lucarneHostile';
const HOSTILE_SCRIPT = `<script>window.${HOSTILE_GLOBAL}=1</script>`;
const HOSTILE_IMG = `<img src="x" onerror="window.${HOSTILE_GLOBAL}=1">`;
/** The output of the hostile tool call, which tries to close the block it is shown in. */
const HOSTILE_OUTPUT = `</pre>${HOSTILE_SCRIPT}`;
const SAFE_HREF = 'https://docs.example.com/page';
const TRACKER_HOST = 'tracker.example.com';
const UNSAFE_LINK_LABELS = ['run me', 'as data', 'vb'];
const UNSAFE_HREFS = 'a[href^="javascript:" i], a[href^="data:" i], a[href^="vbscript:" i]';
/** The only scripts the page may hold: the bundle, from the server. */
const BUNDLE_PATH = '/assets/';

const SECURITY_HEADERS: Record<string, string | RegExp> = {
  'content-security-policy': /default-src 'self'.*script-src 'self'.*object-src 'none'/,
  'x-content-type-options': 'nosniff',
  'referrer-policy': 'no-referrer',
};

/** Whether the hostile scripts ran. */
function hostileRan(page: Page): Promise<boolean> {
  return page.evaluate((name) => name in window, HOSTILE_GLOBAL);
}

describe('the hostile run', () => {
  let browser: Browser;
  let watched: Watched;
  let page: Page;

  beforeAll(async () => {
    browser = await launch();
    watched = await open(browser, (await run('hostile')).link);
    page = watched.page;
    await visible(assistantMessages(page).filter({ hasText: 'Hostile Markdown follows.' }));
  });
  afterAll(async () => {
    await close(watched);
    await browser.close();
  });

  it('is served with its security headers', () => {
    const headers = watched.response?.headers() ?? {};
    for (const [name, want] of Object.entries(SECURITY_HEADERS)) {
      if (typeof want === 'string') expect(headers[name]).toBe(want);
      else expect(headers[name]).toMatch(want);
    }
  });

  it('shows hostile HTML as text', async () => {
    const message = assistantMessages(page).filter({ hasText: 'Hostile Markdown follows.' });
    await visible(message.getByText(HOSTILE_SCRIPT, { exact: true }));
    await visible(message.getByText(HOSTILE_IMG, { exact: true }));
    // A raw line that is only a script tag is shown as plain output.
    await visible(transcript(page).locator('pre').getByText(HOSTILE_SCRIPT, { exact: true }));
  });

  it('shows a tool output that closes its block as text', async () => {
    const tool = transcript(page).getByRole('article', { name: 'Tool call Bash' });
    await tool.locator('summary').first().click();
    await visible(tool.getByText(HOSTILE_OUTPUT));
  });

  it('runs none of its scripts and has no element they could hide in', async () => {
    const t = transcript(page);
    expect(await hostileRan(page)).toBe(false);
    expect(await t.locator('script, img, iframe, object, embed, svg').count()).toBe(0);
    const handlers = await t.evaluate((root) =>
      [...root.querySelectorAll('*')].flatMap((el) =>
        el.getAttributeNames().filter((name) => name.startsWith('on')),
      ),
    );
    expect(handlers).toEqual([]);
    const scripts = await page.evaluate(() => [...document.scripts].map((s) => s.src));
    const bundle = `${new URL(baseURL()).origin}${BUNDLE_PATH}`;
    expect(scripts.length).toBeGreaterThan(0);
    expect(scripts.filter((src) => !src.startsWith(bundle))).toEqual([]);
  });

  it('links only to safe URLs', async () => {
    expect(await page.locator(UNSAFE_HREFS).count()).toBe(0);
    for (const label of UNSAFE_LINK_LABELS) {
      expect(await transcript(page).getByRole('link', { name: label, exact: true }).count()).toBe(
        0,
      );
    }
    const safe = transcript(page).getByRole('link', { name: 'safe', exact: true });
    await visible(safe);
    expect(await safe.getAttribute('href')).toBe(SAFE_HREF);
    expect(await safe.getAttribute('rel')).toContain('noopener');
  });

  it('loads nothing from the tracker or any other origin', async () => {
    expect(await page.locator(`img[src*="${TRACKER_HOST}"]`).count()).toBe(0);
    expect(watched.offOrigin).toEqual([]);
    expect(new URL(page.url()).origin).toBe(new URL(baseURL()).origin);
  });

  it('opens its raw lines without running them', async () => {
    const raw = transcript(page).locator('details.raw');
    const count = await raw.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) await raw.nth(i).locator('summary').click();
    expect(await hostileRan(page)).toBe(false);
    expect(watched.dialogs).toEqual([]);
  });
});
