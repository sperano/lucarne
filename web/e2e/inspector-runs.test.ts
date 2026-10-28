// The inspector over the runs the conversation is checked against: every
// runner fixture lists its attempt's events and its end, the session longer
// than a run retains stays bounded and reachable, the hostile run is shown
// as text and never acts, and a run that went away says so.
import type { Browser, Locator, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Snapshot } from '../src/lib/contract';
import { formatCount } from '../src/lib/format';
import { END_REASON_LABELS, RUN_PHASE_LABELS } from '../src/lib/labels';
import { WINDOW_MAX } from '../src/lib/window';
import {
  claudeInit,
  claudeText,
  deleteTask,
  finish,
  live,
  run,
  runs,
  servedStreams,
  snapshot,
  write,
  type RunInfo,
} from './control';
import {
  close,
  eventDetail,
  eventList,
  eventRow,
  eventually,
  FAILED_RESOURCE,
  INSPECTOR_QUERY,
  inViewport,
  launch,
  open,
  openStreams,
  streamStatus,
  visible,
  type Watched,
} from './harness';

const FIXTURE_PREFIX = 'fixture/';
const LIVE_TEXT = 'Live: new events appear as the log is read.';
const NOT_KNOWN_TEXT = 'This run is not known: it never existed here, or its Task was deleted';
/** The messages the long-session run wrote (scenario.LongSessionMessages). */
const LONG_SESSION_MESSAGES = 25_000;
/** A message late in the session, far outside the window it opens with. */
const LATE_MESSAGE = 24_321;
/** The property the hostile run's scripts set if they ever ran (scenario.HostileGlobal). */
const HOSTILE_GLOBAL = '__lucarneHostile';
const HOSTILE_SCRIPT = `<script>window.${HOSTILE_GLOBAL}=1</script>`;
const UNSAFE_HREFS = 'a[href^="javascript:" i], a[href^="data:" i], a[href^="vbscript:" i]';

const sessionMessage = (n: number): string => `Message ${String(n)} of a long session.`;
const options = (page: Page): Locator => eventList(page).getByRole('option');
const sourceText = (page: Page): Locator =>
  eventDetail(page).getByRole('region', { name: 'Source line' }).locator('pre');
const search = (page: Page): Locator => page.getByRole('searchbox', { name: 'Search the events' });

/** The events of the attempt the inspector opens a run on: its latest. */
function latestAttemptEvents(snap: Snapshot) {
  const latest = snap.attempts.at(-1)?.id;
  return snap.events.filter((e) => e.attempt === latest);
}

const fixtures: RunInfo[] = Object.values(await runs()).filter((r) =>
  r.name.startsWith(FIXTURE_PREFIX),
);

describe('the inspector over fixture runs', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  it.each(fixtures.map((r) => [r.name, r] as const))(
    '%s lists its events and its end',
    async (_, r) => {
      const snap = await snapshot(r);
      const held = latestAttemptEvents(snap);
      const [first] = held;
      if (!first) throw new Error(`${r.name} has no event`);
      const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
      const { page } = watched;
      await visible(options(page));
      expect(await options(page).count()).toBe(Math.min(held.length, WINDOW_MAX));
      const label = RUN_PHASE_LABELS[snap.run.phase];
      await visible(
        streamStatus(page).filter({
          hasText: `${END_REASON_LABELS.run_finished} Task phase: ${label}.`,
        }),
      );
      await eventRow(page, first.id).click();
      await visible(eventDetail(page).getByRole('article'));
      // Its source line, as the runner wrote it.
      await eventually(async () => (await sourceText(page).innerText()).length).toBeGreaterThan(0);
      await close(watched);
    },
  );
});

describe('the inspector over the long-session run', () => {
  let browser: Browser;
  let watched: Watched;
  let page: Page;
  let snap: Snapshot;

  beforeAll(async () => {
    browser = await launch();
    const r = await run('long-session');
    snap = await snapshot(r);
    watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    page = watched.page;
    await visible(options(page));
  });
  afterAll(async () => {
    await close(watched);
    await browser.close();
  });

  it('says how many of its oldest events were dropped', async () => {
    const dropped = snap.stream.droppedEvents;
    expect(dropped).toBeGreaterThan(0);
    expect(snap.events.length + dropped).toBe(LONG_SESSION_MESSAGES + 1);
    await visible(
      page.getByRole('note').filter({
        hasText: `The ${formatCount(dropped)} oldest events of this run were dropped`,
      }),
    );
    await visible(page.getByText(`of ${formatCount(snap.events.length)}`));
  });

  it('reaches its last event by keyboard within a bounded list', async () => {
    const last = snap.events.at(-1);
    if (!last) throw new Error('no event');
    await eventList(page).focus();
    await page.keyboard.press('End');
    await eventually(() => eventRow(page, last.id).getAttribute('aria-selected')).toBe('true');
    await eventually(() => inViewport(eventRow(page, last.id))).toBe(true);
    await visible(eventDetail(page).getByText(sessionMessage(LONG_SESSION_MESSAGES)));
    expect(await options(page).count()).toBeLessThanOrEqual(WINDOW_MAX);
  });

  it('finds a late message from its start', async () => {
    const first = snap.events[0];
    const late = snap.events.find(
      (e) => e.kind === 'message' && e.message.text === sessionMessage(LATE_MESSAGE),
    );
    if (!first || !late) throw new Error('no late message');
    await page.keyboard.press('Home');
    await eventually(() => eventRow(page, first.id).getAttribute('aria-selected')).toBe('true');
    await search(page).fill(sessionMessage(LATE_MESSAGE));
    await visible(page.getByText('1 match', { exact: true }));
    await search(page).press('Enter');
    await visible(page.getByText('1 of 1', { exact: true }));
    await eventually(() => eventRow(page, late.id).getAttribute('aria-selected')).toBe('true');
    await eventually(() => inViewport(eventRow(page, late.id))).toBe(true);
    expect(await options(page).count()).toBeLessThanOrEqual(WINDOW_MAX);
  });

  it('does not find a dropped message', async () => {
    await search(page).fill(sessionMessage(1));
    await visible(page.getByText('No match', { exact: true }));
  });
});

describe('the inspector over the hostile run', () => {
  let browser: Browser;
  let watched: Watched;
  let page: Page;
  let snap: Snapshot;

  beforeAll(async () => {
    browser = await launch();
    const r = await run('hostile');
    snap = await snapshot(r);
    watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    page = watched.page;
    await visible(options(page));
  });
  afterAll(async () => {
    await close(watched);
    await browser.close();
  });

  it('shows every event and its source line as text, running none', async () => {
    for (const e of snap.events) {
      await eventRow(page, e.id).click();
      await eventually(() => new URL(page.url()).hash).toBe(`#event=${e.id}`);
      await visible(eventDetail(page).getByRole('article'));
    }
    const script = snap.events.find((e) => e.kind === 'raw' && e.raw.text === HOSTILE_SCRIPT);
    if (!script) throw new Error('no raw script line');
    await eventRow(page, script.id).click();
    await eventually(() => sourceText(page).innerText()).toBe(HOSTILE_SCRIPT);
    expect(await page.evaluate((name) => name in window, HOSTILE_GLOBAL)).toBe(false);
  });

  it('has no element a script or a handler could hide in, and no unsafe link', async () => {
    const inspector = page.locator('.inspector');
    expect(await inspector.locator('script, img, iframe, object, embed').count()).toBe(0);
    const handlers = await inspector.evaluate((root) =>
      [...root.querySelectorAll('*')].flatMap((el) =>
        el.getAttributeNames().filter((name) => name.startsWith('on')),
      ),
    );
    expect(handlers).toEqual([]);
    expect(await page.locator(UNSAFE_HREFS).count()).toBe(0);
    expect(watched.offOrigin).toEqual([]);
    expect(watched.dialogs).toEqual([]);
  });
});

describe('the inspector over a run that went away', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  it('says so while it is open, keeps its events and lets the stream go', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'Inspected before the Task went.')]);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    const kept = options(page).filter({ hasText: 'Inspected before the Task went.' });
    await visible(kept);
    await deleteTask(r);
    await visible(streamStatus(page).filter({ hasText: END_REASON_LABELS.run_deleted }));
    await visible(kept);
    await eventually(() => openStreams(page)).toBe(0);
    await close(watched);
    await eventually(() => servedStreams()).toBe(0);
  });

  it('is not known when its link is opened after the Task was deleted', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'A finished run, then deleted.')]);
    await finish(r, 0, 'Succeeded');
    await deleteTask(r);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await visible(page.getByRole('alert').filter({ hasText: NOT_KNOWN_TEXT }));
    expect(await eventList(page).count()).toBe(0);
    await close(watched, FAILED_RESOURCE);
  });
});
