// A live run followed from its first line to its end: lines appear as
// they are written, the view follows the tail, a broken log is read again
// without repeating a line, a rotated one shows its gap, and the end of
// the run closes the stream.
import type { Browser, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { END_REASON_LABELS, GAP_REASON_LABELS, RUN_PHASE_LABELS } from '../src/lib/labels';
import {
  breakLog,
  claudeInit,
  claudeText,
  claudeTexts,
  finish,
  live,
  rotate,
  write,
  type RunInfo,
} from './control';
import {
  assistantMessages,
  close,
  eventually,
  hidden,
  inViewport,
  launch,
  messageTexts,
  open,
  openStreams,
  streamStatus,
  transcript,
  visible,
  type Watched,
} from './harness';

const LIVE_TEXT = 'Live: new events appear as the log is read.';
/** Messages enough to fill more than the viewport, so that following has to scroll. */
const OVERFLOW_MESSAGES = 40;
/** Messages written while the reader has scrolled away. */
const UNREAD_MESSAGES = 3;
/** A paragraph long enough to take a few lines on screen. */
const FILLER = ' Filler to make the message take some room on the page.'.repeat(4);

describe('a live run', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  /** A new live run, opened and live, with nothing but its init line. */
  async function openLive(): Promise<{ r: RunInfo; watched: Watched; page: Page }> {
    const r = await live();
    await write(r, [claudeInit()]);
    const watched = await open(browser, r.link);
    await visible(streamStatus(watched.page).filter({ hasText: LIVE_TEXT }));
    return { r, watched, page: watched.page };
  }

  /** Waits for the last of texts to show, then checks the messages are exactly texts. */
  async function shows(page: Page, texts: readonly string[]): Promise<void> {
    const last = texts.at(-1);
    if (last !== undefined) await visible(assistantMessages(page).filter({ hasText: last }));
    await eventually(() => messageTexts(page)).toEqual(texts);
  }

  it('shows lines as they are written, in order, without a reload', async () => {
    const { r, watched, page } = await openLive();
    await page.evaluate(() => {
      document.body.dataset.e2eLoaded = 'once';
    });
    const texts = ['First live line.', 'Second live line.', 'Third live line.'];
    await write(r, [claudeText(1, texts[0] ?? '')]);
    await shows(page, texts.slice(0, 1));
    await write(
      r,
      texts.slice(1).map((t, i) => claudeText(i + 2, t)),
    );
    await shows(page, texts);
    // The page that shows them is the one loaded first.
    expect(await page.evaluate(() => document.body.dataset.e2eLoaded)).toBe('once');
    await close(watched);
  });

  it('follows the tail, pauses when the reader scrolls up, and follows again', async () => {
    const { r, watched, page } = await openLive();
    const text = (n: number): string => `Overflow message ${String(n)}.${FILLER}`;
    await write(r, claudeTexts(1, OVERFLOW_MESSAGES, text));
    const newest = assistantMessages(page).filter({
      hasText: `Overflow message ${String(OVERFLOW_MESSAGES)}.`,
    });
    await visible(newest);
    const following = page.getByRole('button', { name: 'Following new items' });
    await expect(following.getAttribute('aria-pressed')).resolves.toBe('true');
    await eventually(() => inViewport(newest)).toBe(true);

    // The reader scrolls up: following pauses, and what arrives is counted, not shown.
    await page.evaluate(() => {
      window.scrollTo(0, 0);
    });
    const paused = page.getByRole('button', { name: /^Follow new items/ });
    await visible(paused);
    await expect(paused.getAttribute('aria-pressed')).resolves.toBe('false');
    const first = OVERFLOW_MESSAGES + 1;
    await write(r, claudeTexts(first, UNREAD_MESSAGES, text));
    await visible(
      page.getByRole('button', { name: `Follow new items (${String(UNREAD_MESSAGES)} new)` }),
    );
    const latest = assistantMessages(page).filter({
      hasText: `Overflow message ${String(first + UNREAD_MESSAGES - 1)}.`,
    });
    // The view stays where the reader is.
    expect(await inViewport(assistantMessages(page).first())).toBe(true);

    // Following again brings the newest into view.
    await paused.click();
    await visible(following);
    await eventually(() => inViewport(latest)).toBe(true);
    await close(watched);
  });

  it('reads a broken log again without repeating a message', async () => {
    const { r, watched, page } = await openLive();
    await write(r, [claudeText(1, 'Before the break.'), claudeText(2, 'Also before.')]);
    await shows(page, ['Before the break.', 'Also before.']);
    await breakLog(r);
    await write(r, [claudeText(3, 'After the break.')]);
    await shows(page, ['Before the break.', 'Also before.', 'After the break.']);
    await close(watched);
  });

  it('shows a gap where a rotated log lost lines', async () => {
    const { r, watched, page } = await openLive();
    await write(r, [claudeText(1, 'Before the rotation.')]);
    await shows(page, ['Before the rotation.']);
    await rotate(r);
    await breakLog(r);
    await write(r, [claudeText(2, 'After the rotation.')]);
    await shows(page, ['Before the rotation.', 'After the rotation.']);
    const gaps = transcript(page).getByRole('note').filter({ hasText: 'missing here' });
    await visible(gaps);
    const gapText = await gaps.first().innerText();
    expect(Object.values(GAP_REASON_LABELS).some((reason) => gapText.includes(reason))).toBe(true);
    await visible(page.getByText(/This transcript is incomplete: \d+ gaps?/));
    await close(watched);
  });

  it('shows the end of the run and closes the stream', async () => {
    const { r, watched, page } = await openLive();
    await write(r, [claudeText(1, 'The last line.')]);
    await shows(page, ['The last line.']);
    await finish(r, 1, 'Failed');
    const label = RUN_PHASE_LABELS.failed;
    await visible(
      streamStatus(page).filter({
        hasText: `${END_REASON_LABELS.run_finished} Task phase: ${label}.`,
      }),
    );
    await visible(page.locator('.run .phase').filter({ hasText: label }));
    await visible(page.getByText('exit code 1'));
    await eventually(() => openStreams(page)).toBe(0);
    // Nothing more can come, so the follow control goes.
    await hidden(page.getByRole('button', { name: /Follow/ }));
    await close(watched);
  });
});
