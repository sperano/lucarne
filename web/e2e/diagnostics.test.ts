// The inspector's diagnostics over the real source, stream and server: the
// source line of unknown and malformed records, each attempt's own lines,
// and a live run whose selection, filters and search hold while events
// arrive, a line repeats and the stream breaks and resumes.
import type { Browser, Locator, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import {
  claudeInit,
  claudeText,
  cutStreams,
  live,
  restoreStreams,
  run,
  snapshot,
  write,
} from './control';
import {
  close,
  eventDetail,
  eventList,
  eventRow,
  eventually,
  INSPECTOR_QUERY,
  launch,
  open,
  streamStatus,
  visible,
} from './harness';

const LIVE_TEXT = 'Live: new events appear as the log is read.';
const RECONNECTING_TEXT = 'The live stream broke: reconnecting from the last event received.';

const options = (page: Page): Locator => eventList(page).getByRole('option');
const filters = (page: Page): Locator =>
  page.getByRole('search', { name: 'Search and filter the events' });
const sourceText = (page: Page): Locator =>
  eventDetail(page).getByRole('region', { name: 'Source line' }).locator('pre');
const kind = (page: Page, label: string): Locator =>
  filters(page).locator('.kind').filter({ hasText: label }).getByRole('checkbox');

describe('the inspector’s diagnostics', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterEach(async () => {
    await restoreStreams();
  });
  afterAll(async () => {
    await browser.close();
  });

  it('shows the source line of unknown records and malformed input', async () => {
    const r = await run('fixture/claude-code/malformed-lines');
    const snap = await snapshot(r);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await kind(page, 'Raw line').check();
    const raws = snap.events.filter((e) => e.kind === 'raw');
    await eventually(() => options(page).count()).toBe(raws.length);
    for (const reason of ['malformed_json', 'unknown_type'] as const) {
      const raw = raws.find((e) => e.raw.reason === reason);
      if (!raw) throw new Error(`no ${reason} event`);
      await eventRow(page, raw.id).click();
      const formatted = eventDetail(page).getByLabel('Formatted');
      await visible(sourceText(page));
      if (await formatted.isVisible()) await formatted.uncheck();
      // The line as the runner wrote it: the raw event holds the same text, uncut.
      await eventually(() => sourceText(page).innerText()).toBe(raw.raw.text);
    }
    expect(new URL(page.url()).hash).toBe(
      `#event=${raws.find((e) => e.raw.reason === 'unknown_type')?.id ?? ''}`,
    );
    await close(watched);
  });

  it('lists each attempt’s own events and source lines', async () => {
    const r = await run('retried');
    const snap = await snapshot(r);
    const [first, second] = snap.attempts;
    if (!first || !second) throw new Error('the run has no two attempts');
    const ofAttempt = (id: string) => snap.events.filter((e) => e.attempt === id);
    const watched = await open(browser, `${r.link}?attempt=${first.id}&${INSPECTOR_QUERY}`);
    const { page } = watched;
    await eventually(() => options(page).count()).toBe(ofAttempt(first.id).length);
    const firstEvent = ofAttempt(first.id).at(-1);
    if (!firstEvent) throw new Error('the first attempt has no event');
    await eventRow(page, firstEvent.id).click();
    await visible(sourceText(page));

    await page.getByRole('combobox', { name: /^Attempt/ }).selectOption(second.id);
    await eventually(() => options(page).count()).toBe(ofAttempt(second.id).length);
    for (const id of await options(page).evaluateAll((rows) =>
      rows.map((r) => r.getAttribute('data-event')),
    )) {
      expect(id?.startsWith(second.id)).toBe(true);
    }
    await close(watched);
  });

  it('keeps the selection, filters and search through live events, a repeated line and a reconnect', async () => {
    const r = await live();
    const repeated = claudeText(3, 'needle repeated');
    await write(r, [claudeInit(), claudeText(1, 'needle one')]);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await kind(page, 'Message').check();
    await filters(page).getByRole('searchbox').fill('needle');
    await eventually(() => options(page).count()).toBe(1);
    await options(page).first().click();
    const selected = await options(page).first().getAttribute('data-event');
    await visible(sourceText(page).filter({ hasText: 'needle one' }));

    // A line written twice is two events, both listed and both matched.
    await write(r, [claudeText(2, 'not a match'), repeated, repeated]);
    await eventually(() => options(page).count()).toBe(4);
    await eventually(() => eventList(page).locator('.match').count()).toBe(3);

    await cutStreams();
    await visible(streamStatus(page).filter({ hasText: RECONNECTING_TEXT }));
    await write(r, [claudeText(4, 'needle after the outage')]);
    await restoreStreams();
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await eventually(() => options(page).count()).toBe(5);
    await visible(options(page).filter({ hasText: 'needle after the outage' }));

    expect(await eventList(page).locator('[aria-selected=true]').getAttribute('data-event')).toBe(
      selected,
    );
    expect(new URL(page.url()).hash).toBe(`#event=${selected ?? ''}`);
    await expect.poll(() => kind(page, 'Message').isChecked()).toBe(true);
    await filters(page).getByRole('button', { name: 'Next' }).click();
    await eventually(() => filters(page).innerText()).toContain('2 of 4');
    await close(watched);
  });
});
