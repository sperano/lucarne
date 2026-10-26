// A run Kelos retried: each of its pods is an attempt, the latest shown
// first, and the reader switches between them without mixing their lines.
import type { Browser } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { run, snapshot, type RunInfo } from './control';
import { assistantMessages, close, eventually, launch, open, visible } from './harness';

// The messages of its attempts, as in internal/e2e/scenario/runs.go.
const FIRST_TEXT = 'First attempt: the tests fail.';
const SECOND_TEXT = 'Second attempt: the tests pass.';

describe('a retried run', () => {
  let browser: Browser;
  let r: RunInfo;
  let firstAttempt: string;

  beforeAll(async () => {
    browser = await launch();
    r = await run('retried');
    const snap = await snapshot(r);
    expect(snap.attempts).toHaveLength(2);
    firstAttempt = snap.attempts[0]?.id ?? '';
  });
  afterAll(async () => {
    await browser.close();
  });

  it('shows its latest attempt, then the one the reader picks, and links to it', async () => {
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(assistantMessages(page).filter({ hasText: SECOND_TEXT }));
    await eventually(() => assistantMessages(page).filter({ hasText: FIRST_TEXT }).count()).toBe(0);

    await page.getByLabel('Attempt').selectOption(firstAttempt);
    await visible(assistantMessages(page).filter({ hasText: FIRST_TEXT }));
    await eventually(() => assistantMessages(page).filter({ hasText: SECOND_TEXT }).count()).toBe(
      0,
    );
    expect(new URL(page.url()).searchParams.get('attempt')).toBe(firstAttempt);

    await page.reload();
    await visible(assistantMessages(page).filter({ hasText: FIRST_TEXT }));
    await eventually(() => assistantMessages(page).filter({ hasText: SECOND_TEXT }).count()).toBe(
      0,
    );
    await close(watched);
  });
});
