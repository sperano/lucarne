// The home page searches the runs the server lists: the allowlisted
// namespace's, never the private one's, and a match opens its run.
import type { Browser } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { run, type RunInfo } from './control';
import { close, eventually, launch, open, visible } from './harness';

/** The Task name of the hostile run, as internal/e2e/scenario names it. */
const HOSTILE_TASK = 'example-hostile';
/** The UID that ends a run link. */
const LINK_UID = /\/([^/]+)$/;

describe('the run search', () => {
  let browser: Browser;
  let hostile: RunInfo;
  let privateUID: string;

  beforeAll(async () => {
    browser = await launch();
    hostile = await run('hostile');
    privateUID = LINK_UID.exec((await run('private')).link)?.[1] ?? '';
    expect(privateUID).not.toBe('');
  });
  afterAll(async () => {
    await browser.close();
  });

  it('finds a run by its Task name and opens it on Enter', async () => {
    const watched = await open(browser, '/');
    const { page } = watched;
    const results = page.getByRole('list', { name: 'Matching runs' }).getByRole('link');
    await visible(results.first());
    expect(await page.content()).not.toContain(privateUID);

    await page.getByLabel('Search the runs').fill(HOSTILE_TASK);
    await eventually(() => results.count()).toBe(1);
    expect(await results.first().getAttribute('href')).toBe(hostile.link);
    expect(new URL(page.url()).searchParams.get('q')).toBe(HOSTILE_TASK);

    await page.keyboard.press('Enter');
    await visible(page.getByRole('heading', { name: HOSTILE_TASK }));
    expect(new URL(page.url()).pathname).toBe(hostile.link);

    await page.getByRole('link', { name: 'Lucarne' }).click();
    await visible(page.getByLabel('Search the runs'));
    await close(watched);
  });
});
