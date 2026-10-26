// Runs whose Task or pod went away: the page says so explicitly, keeps
// what it had loaded, and a link to a deleted run says it is not known.
import type { Browser } from 'playwright';
import { afterAll, beforeAll, describe, it } from 'vitest';
import { END_REASON_LABELS } from '../src/lib/labels';
import { claudeInit, claudeText, deleteTask, finish, live, run, write } from './control';
import {
  assistantMessages,
  close,
  eventually,
  FAILED_RESOURCE,
  launch,
  open,
  openStreams,
  streamStatus,
  visible,
} from './harness';

const LIVE_TEXT = 'Live: new events appear as the log is read.';
/** What the page says of a run the server no longer has (RunPage.svelte). */
const NOT_KNOWN_TEXT = 'This run is not known: it never existed here, or its Task was deleted';
/** What the attempt's coverage says once its log is gone (Completeness.svelte). */
const EXPIRED_TEXT = 'The log of this attempt is gone';

describe('a run that went away', () => {
  let browser: Browser;

  beforeAll(async () => {
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  it('says so while it is open, and keeps what was loaded', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'Read before the Task went.')]);
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(streamStatus(page).filter({ hasText: LIVE_TEXT }));
    await visible(assistantMessages(page).filter({ hasText: 'Read before the Task went.' }));
    await deleteTask(r);
    await visible(streamStatus(page).filter({ hasText: END_REASON_LABELS.run_deleted }));
    await visible(page.locator('.run .phase').filter({ hasText: 'Task deleted' }));
    await visible(page.getByRole('note').filter({ hasText: EXPIRED_TEXT }));
    await visible(assistantMessages(page).filter({ hasText: 'Read before the Task went.' }));
    await eventually(() => openStreams(page)).toBe(0);
    await close(watched);
  });

  it('is not known when its link is opened after the Task was deleted', async () => {
    const r = await live();
    await write(r, [claudeInit(), claudeText(1, 'A finished run, then deleted.')]);
    await finish(r, 0, 'Succeeded');
    await deleteTask(r);
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(page.getByRole('alert').filter({ hasText: NOT_KNOWN_TEXT }));
    await eventually(() => assistantMessages(page).count()).toBe(0);
    await close(watched, FAILED_RESOURCE);
  });

  it('says it has no attempt when its pod is gone', async () => {
    const r = await run('pod-deleted');
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(
      page.getByRole('note').filter({ hasText: 'This run lists no attempt: its pod is gone' }),
    );
    await eventually(() => page.getByText('has not been created').count()).toBe(0);
    await eventually(() => assistantMessages(page).count()).toBe(0);
    await close(watched);
  });
});
