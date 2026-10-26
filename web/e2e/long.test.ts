// Output past Lucarne's bounds: an oversized line and cut fields are
// marked as such, and a session longer than a run retains says what it
// dropped, keeps a bounded DOM, reaches its last message and finds a late
// one.
import type { Browser, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { formatCount } from '../src/lib/format';
import { RAW_REASON_LABELS } from '../src/lib/labels';
import { WINDOW_MAX, WINDOW_STEP } from '../src/lib/window';
import { run, snapshot } from './control';
import {
  assistantMessages,
  close,
  eventually,
  inViewport,
  launch,
  open,
  renderedItems,
  transcript,
  visible,
  type Watched,
} from './harness';

/** The messages the long-session run wrote (scenario.LongSessionMessages). */
const LONG_SESSION_MESSAGES = 25_000;
/** A message late in the session, far outside the window it opens with. */
const LATE_MESSAGE = 24_321;
/** "Show later items" presses enough to grow the window past WINDOW_MAX. */
const LATER_PRESSES = WINDOW_MAX / WINDOW_STEP + 2;

const sessionMessage = (n: number): string => `Message ${String(n)} of a long session.`;

describe('the long-line run', () => {
  let browser: Browser;
  let watched: Watched;
  let page: Page;

  beforeAll(async () => {
    browser = await launch();
    watched = await open(browser, (await run('long-line')).link);
    page = watched.page;
    await visible(assistantMessages(page).filter({ hasText: 'Done after the long output.' }));
  });
  afterAll(async () => {
    await close(watched);
    await browser.close();
  });

  it('marks the line over the size limit', async () => {
    await visible(
      transcript(page).locator('summary').filter({ hasText: RAW_REASON_LABELS.oversized }),
    );
    await visible(
      page.getByText('This transcript is incomplete: 1 line cut to the line size limit'),
    );
  });

  it('marks the message and the tool output cut to the event limits', async () => {
    const cut = transcript(page).getByText('Cut to the size limit by Lucarne:');
    // The raw line's text, the long message's text and the tool output.
    await eventually(() => cut.count()).toBe(3);
    await visible(cut.filter({ hasText: /^Cut to the size limit by Lucarne: text \(/ }));
    const tool = transcript(page).getByRole('article', { name: 'Tool call Bash' });
    await tool.locator('summary').first().click();
    await visible(tool.getByText(/Cut to the size limit by Lucarne: output \(/));
  });
});

describe('the long-session run', () => {
  let browser: Browser;
  let watched: Watched;
  let page: Page;
  let retained: number;
  let dropped: number;

  beforeAll(async () => {
    browser = await launch();
    const r = await run('long-session');
    const snap = await snapshot(r);
    retained = snap.events.length;
    dropped = snap.stream.droppedEvents;
    watched = await open(browser, r.link);
    page = watched.page;
    await visible(transcript(page));
  });
  afterAll(async () => {
    await close(watched);
    await browser.close();
  });

  it('says how many of its oldest events were dropped', async () => {
    expect(dropped).toBeGreaterThan(0);
    expect(retained + dropped).toBe(LONG_SESSION_MESSAGES + 1);
    await visible(
      page.getByRole('note').filter({
        hasText: `The ${formatCount(dropped)} oldest events of this run were dropped`,
      }),
    );
    await visible(page.getByText(`not the ${formatCount(dropped)} oldest events of the run`));
    // The transcript starts after them.
    await visible(
      assistantMessages(page)
        .first()
        .filter({ hasText: sessionMessage(LONG_SESSION_MESSAGES - retained + 1) }),
    );
  });

  it('renders a bounded window of its items', async () => {
    await visible(page.getByText(`of ${formatCount(retained)}`));
    expect(await renderedItems(page).count()).toBeLessThanOrEqual(WINDOW_STEP);
    const later = page.getByRole('button', { name: /^Show later items/ });
    for (let i = 0; i < LATER_PRESSES; i++) await later.click();
    const count = await renderedItems(page).count();
    expect(count).toBeGreaterThan(WINDOW_STEP);
    expect(count).toBeLessThanOrEqual(WINDOW_MAX);
  });

  it('jumps to its last message', async () => {
    await page.getByRole('button', { name: 'End', exact: true }).click();
    const last = assistantMessages(page).filter({ hasText: sessionMessage(LONG_SESSION_MESSAGES) });
    await visible(last);
    await expect(page.getByText(/^Items .* of /).innerText()).resolves.toMatch(
      new RegExp(`–${formatCount(retained)} of ${formatCount(retained)}$`),
    );
    // End brings it into view, and keeps it there while the items above it
    // grow from their placeholder height to their real one.
    await eventually(() => inViewport(last)).toBe(true);
    expect(await renderedItems(page).count()).toBeLessThanOrEqual(WINDOW_MAX);
  });

  it('finds a late message from its start', async () => {
    await page.getByRole('button', { name: 'Start', exact: true }).click();
    const search = page.getByRole('searchbox', { name: 'Search the transcript' });
    await search.fill(sessionMessage(LATE_MESSAGE));
    await visible(page.getByText('1 match', { exact: true }));
    await search.press('Enter');
    await visible(page.getByText('1 of 1', { exact: true }));
    const found = assistantMessages(page).filter({ hasText: sessionMessage(LATE_MESSAGE) });
    await visible(found);
    // The result is marked as the one shown, and stays in view while the items
    // around it grow from their placeholder height to their real one.
    const item = renderedItems(page).filter({ hasText: sessionMessage(LATE_MESSAGE) });
    await eventually(() => item.getAttribute('class')).toMatch(/\bcurrent\b/);
    await eventually(() => inViewport(found)).toBe(true);
    expect(await renderedItems(page).count()).toBeLessThanOrEqual(WINDOW_MAX);
  });

  it('does not find a dropped message', async () => {
    const search = page.getByRole('searchbox', { name: 'Search the transcript' });
    await search.fill(sessionMessage(1));
    await visible(page.getByText('No match', { exact: true }));
  });
});
