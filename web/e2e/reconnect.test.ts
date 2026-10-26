// A live stream that breaks while lines are written: the page says it
// reconnects, its EventSource resumes from the last event it received,
// and every line written meanwhile shows exactly once.
//
// Chromium's offline emulation (context.setOffline) leaves an open
// EventSource connected and delivering, so the outage is made on the
// server: lucarne-e2e's /_e2e/streams/cut ends the open streams and holds
// the reconnects until /_e2e/streams/restore.
import type { Browser, Request } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { claudeInit, claudeText, cutStreams, live, restoreStreams, write } from './control';
import {
  assistantMessages,
  close,
  eventually,
  launch,
  messageTexts,
  open,
  streamStatus,
  visible,
} from './harness';

const LIVE_TEXT = 'Live: new events appear as the log is read.';
const RECONNECTING_TEXT = 'The live stream broke: reconnecting from the last event received.';
const EVENTS_SUFFIX = '/events';

describe('a live stream that breaks', () => {
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

  it('reconnects and shows what was written meanwhile exactly once', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'Before the outage.')]);
    const watched = await open(browser, r.link);
    const { page } = watched;
    const streams: Request[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname.endsWith(EVENTS_SUFFIX)) streams.push(request);
    });
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await visible(assistantMessages(page).filter({ hasText: 'Before the outage.' }));

    const opened = streams.length;
    await cutStreams();
    await visible(streamStatus(page).filter({ hasText: RECONNECTING_TEXT }));
    const during = ['Written during the outage.', 'Also during the outage.'];
    await write(
      r,
      during.map((t, i) => claudeText(i + 2, t)),
    );
    // The browser tries again on its own, and the server holds it.
    await eventually(() => streams.length).toBeGreaterThan(opened);
    expect(await messageTexts(page)).toEqual(['Before the outage.']);

    await restoreStreams();
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await visible(assistantMessages(page).filter({ hasText: during.at(-1) ?? '' }));
    await eventually(() => messageTexts(page)).toEqual(['Before the outage.', ...during]);
    // It resumed from the last event it had, rather than loading the run again.
    const retries = streams.slice(opened);
    const resumed = await Promise.all(retries.map((s) => s.headerValue('last-event-id')));
    expect(resumed.every((id) => id !== null && id !== '')).toBe(true);
    await close(watched);
  });
});
