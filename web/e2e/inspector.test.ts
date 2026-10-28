// The inspector over the real source, stream and server: every runner's
// failing tools expand from the event list, the keyboard moves through it,
// a live run's events arrive in it, and switching back to the conversation
// keeps the run and the selected event.
import type { Browser } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Snapshot } from '../src/lib/contract';
import { claudeInit, claudeTexts, live, run, snapshot, write } from './control';
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
  transcript,
  visible,
} from './harness';

const RUNNERS = ['claude-code', 'codex', 'opencode'] as const;
const LIVE_TEXT = 'Live: new events appear as the log is read.';
/** Messages a test writes to the live run. */
const LIVE_MESSAGES = 5;

/** The first call of snap whose result failed, and that result. */
function failedCall(snap: Snapshot) {
  for (const result of snap.events) {
    if (result.kind !== 'tool_result' || result.toolResult.status !== 'error') continue;
    const call = snap.events.find(
      (e) => e.kind === 'tool_call' && e.toolCall.callId === result.toolResult.callId,
    );
    if (call?.kind === 'tool_call') return { call, result };
  }
  throw new Error(`run ${snap.run.taskName} has no failed call`);
}

describe('the inspector', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  it.each(RUNNERS)('expands a failed %s command from the event list', async (runner) => {
    const r = await run(`fixture/${runner}/tool-failures`);
    const snap = await snapshot(r);
    const { call, result } = failedCall(snap);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await visible(eventRow(page, call.id));
    expect(await eventList(page).getByRole('option').count()).toBe(snap.events.length);

    await eventRow(page, call.id).click();
    await eventually(() => eventRow(page, call.id).getAttribute('aria-selected')).toBe('true');
    const card = eventDetail(page).getByRole('article', {
      name: `Tool call ${call.toolCall.name}`,
    });
    await visible(card.locator('.status').filter({ hasText: 'error' }));
    const output = result.toolResult.output?.split('\n')[0];
    if (output !== undefined) await visible(card.getByText(output, { exact: false }));
    expect(new URL(page.url()).hash).toBe(`#event=${call.id}`);
    await close(watched);
  });

  it('moves with the keyboard and keeps the event in the conversation', async () => {
    const r = await run('fixture/claude-code/tool-failures');
    const snap = await snapshot(r);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    const [first, second] = snap.events;
    if (!first || !second) throw new Error('too few events');
    await visible(eventRow(page, first.id));
    await eventList(page).focus();
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await eventually(() => eventRow(page, second.id).getAttribute('aria-selected')).toBe('true');
    await page.keyboard.press('Enter');
    await eventually(() => eventDetail(page).evaluate((e) => e === document.activeElement)).toBe(
      true,
    );

    await page
      .getByRole('navigation', { name: 'View' })
      .getByRole('link', { name: 'Conversation' })
      .click();
    await visible(transcript(page).locator('.targeted'));
    expect(new URL(page.url()).search).toBe('');
    expect(new URL(page.url()).hash).toBe(`#event=${second.id}`);
    await close(watched);
  });

  it('lists a live run’s events as they are written', async () => {
    const r = await live();
    await write(r, [claudeInit()]);
    const watched = await open(browser, `${r.link}?${INSPECTOR_QUERY}`);
    const { page } = watched;
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    const before = await eventList(page).getByRole('option').count();
    await write(
      r,
      claudeTexts(1, LIVE_MESSAGES, (n) => `Inspected step ${String(n)}`),
    );
    await eventually(() => eventList(page).getByRole('option').count()).toBe(
      before + LIVE_MESSAGES,
    );
    await visible(
      eventList(page)
        .getByRole('option')
        .filter({ hasText: `Inspected step ${String(LIVE_MESSAGES)}` }),
    );
    await close(watched);
  });
});
