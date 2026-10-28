// Both views over one live run, as Gordon's links open it: the conversation
// is the default, the inspector a switch away, and switching back and forth
// in place loads no second snapshot and opens no second stream; the server
// serves one stream for the page until the page goes.
import type { Browser, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import {
  claudeInit,
  claudeText,
  claudeTexts,
  live,
  run,
  servedStreams,
  snapshot,
  write,
} from './control';
import {
  assistantMessages,
  close,
  eventDetail,
  eventList,
  eventRow,
  eventually,
  focused,
  INSPECTOR_QUERY,
  inViewport,
  launch,
  open,
  openStreams,
  streamStatus,
  transcript,
  viewLink,
  visible,
} from './harness';

const LIVE_TEXT = 'Live: new events appear as the log is read.';
const EVENTS_SUFFIX = '/events';
/** Round trips conversation → inspector → conversation a test makes. */
const ROUND_TRIPS = 3;
/** Messages written to the live run after the switches. */
const LATER_MESSAGES = 3;

/** Every EventSource the page created, open or closed. */
function streamsCreated(page: Page): Promise<number> {
  return page.evaluate(() => (window.__e2eStreams ?? []).length);
}

describe('the two views', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  it('opens a Gordon link in the conversation, on the attempt it names', async () => {
    const r = await run('retried');
    const snap = await snapshot(r);
    const [first] = snap.attempts;
    if (!first) throw new Error('the run has no attempt');
    const link = `${r.link}?attempt=${first.id}`;
    const watched = await open(browser, link);
    const { page } = watched;
    await visible(transcript(page));
    expect(await viewLink(page, 'Conversation').getAttribute('aria-current')).toBe('page');
    expect(await eventList(page).count()).toBe(0);
    await eventually(() => page.getByLabel('Attempt').inputValue()).toBe(first.id);
    const url = new URL(page.url());
    expect(`${url.pathname}${url.search}`).toBe(link);
    // The inspector link keeps the attempt.
    const inspector = new URL(
      (await viewLink(page, 'Inspector').getAttribute('href')) ?? '',
      page.url(),
    );
    expect(inspector.searchParams.get('attempt')).toBe(first.id);
    expect(inspector.searchParams.get('view')).toBe('inspector');
    await close(watched);
  });

  it('switches in place on one snapshot and one stream', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'Before the switches.')]);
    const before = await servedStreams();
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await visible(assistantMessages(page).filter({ hasText: 'Before the switches.' }));
    const history = await page.evaluate(() => window.history.length);

    for (let i = 0; i < ROUND_TRIPS; i++) {
      await viewLink(page, 'Inspector').click();
      await visible(eventList(page));
      expect(new URL(page.url()).search).toBe(`?${INSPECTOR_QUERY}`);
      await viewLink(page, 'Conversation').click();
      await visible(transcript(page));
      expect(new URL(page.url()).search).toBe('');
    }
    await write(
      r,
      claudeTexts(2, LATER_MESSAGES, (n) => `After the switches ${String(n)}`),
    );
    const last = `After the switches ${String(1 + LATER_MESSAGES)}`;
    await visible(assistantMessages(page).filter({ hasText: last }));
    await viewLink(page, 'Inspector').click();
    await visible(eventList(page).getByRole('option').filter({ hasText: last }));

    expect(watched.requests.filter((path) => path === r.api)).toHaveLength(1);
    expect(watched.requests.filter((path) => path === `${r.api}${EVENTS_SUFFIX}`)).toHaveLength(1);
    expect(await streamsCreated(page)).toBe(1);
    expect(await openStreams(page)).toBe(1);
    expect(await servedStreams()).toBe(before + 1);
    expect(await page.evaluate(() => window.history.length)).toBe(history);
    await close(watched);
    // The server lets the stream go with the page.
    await eventually(() => servedStreams()).toBe(before);
  });

  it('keeps the event, by keyboard, from one view to the other and on reload', async () => {
    const r = await run('fixture/claude-code/tool-failures');
    const snap = await snapshot(r);
    const call = snap.events.find((e) => e.kind === 'tool_call');
    const last = snap.events.at(-1);
    if (!call || !last) throw new Error('the run has no tool call');
    const watched = await open(browser, `${r.link}#event=${call.id}`);
    const { page } = watched;
    await visible(transcript(page).locator('.targeted'));

    await viewLink(page, 'Inspector').focus();
    await page.keyboard.press('Enter');
    // The linked event is selected and the list has the focus, so the keys work at once.
    await eventually(() => eventRow(page, call.id).getAttribute('aria-selected')).toBe('true');
    await eventually(() => focused(eventList(page))).toBe(true);
    await page.keyboard.press('End');
    await eventually(() => eventRow(page, last.id).getAttribute('aria-selected')).toBe('true');
    expect(new URL(page.url()).hash).toBe(`#event=${last.id}`);
    await page.keyboard.press('Enter');
    await eventually(() => focused(eventDetail(page))).toBe(true);
    await page.keyboard.press('Escape');
    await eventually(() => focused(eventList(page))).toBe(true);

    await page.reload();
    await eventually(() => eventRow(page, last.id).getAttribute('aria-selected')).toBe('true');
    expect(new URL(page.url()).search).toBe(`?${INSPECTOR_QUERY}`);

    await viewLink(page, 'Conversation').focus();
    await page.keyboard.press('Enter');
    const targeted = transcript(page).locator('.targeted');
    await visible(targeted);
    await eventually(() => inViewport(targeted)).toBe(true);
    expect(new URL(page.url()).hash).toBe(`#event=${last.id}`);
    expect(new URL(page.url()).search).toBe('');
    await close(watched);
  });
});
