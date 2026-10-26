// Links that try to reach what Lucarne must not read: a run outside the
// namespace allowlist, its UID under an allowlisted namespace, malformed
// and encoded paths, and a pod labelled for a run that its Job does not
// own. None shows a transcript, and neither run's line ever shows.
import type { Browser } from 'playwright';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { get, run, snapshot } from './control';
import {
  assistantMessages,
  close,
  eventually,
  FAILED_RESOURCE,
  FOREIGN_MARKER,
  launch,
  open,
  PRIVATE_MARKER,
  transcript,
  visible,
} from './harness';

const NOT_KNOWN_TEXT = 'This run is not known: it never existed here, or its Task was deleted';
const INVALID_TEXT = 'This is not a valid run link';
const NOT_FOUND = 404;
const BAD_REQUEST = 400;
/** The UID that ends a run link. */
const LINK_UID = /\/([^/]+)$/;
/** A Kubernetes UID. */
const UID_PATTERN = /^[0-9a-f-]{36}$/;

const privateRun = await run('private');
const privateUID = LINK_UID.exec(privateRun.link)?.[1] ?? '';

describe('links that try to escape', () => {
  let browser: Browser;

  beforeAll(async () => {
    expect(privateUID).toMatch(UID_PATTERN);
    browser = await launch();
  });
  afterAll(async () => {
    await browser.close();
  });

  /** Opens path and checks the page says alert, with no transcript. */
  async function refused(path: string, alert: string): Promise<void> {
    const watched = await open(browser, path);
    const { page } = watched;
    await visible(page.getByRole('alert').filter({ hasText: alert }));
    await eventually(() => transcript(page).count()).toBe(0);
    await close(watched, FAILED_RESOURCE);
  }

  it('does not find the run of a namespace outside the allowlist', async () => {
    await refused(privateRun.link, NOT_KNOWN_TEXT);
  });

  it('does not find that run by its UID under an allowlisted namespace', async () => {
    await refused(`/runs/example/agents/${privateUID}`, NOT_KNOWN_TEXT);
  });

  it('does not follow a dot segment into another namespace', async () => {
    // The browser resolves the dot segment: this is the private run's own link.
    await refused(`/runs/example/agents/../private/${privateUID}`, NOT_KNOWN_TEXT);
  });

  it.each([
    ['a malformed UID', '/runs/example/agents/not-a-uid'],
    ['an encoded slash', `/runs/example/..%2Fprivate/${privateUID}`],
    ['an encoded namespace', `/runs/example/%70rivate/${privateUID}`],
    ['too many segments', `/runs/example/agents/private/${privateUID}`],
  ])('refuses %s', async (_, path) => {
    await refused(path, INVALID_TEXT);
  });

  it('answers the API for those paths with errors that hold no line', async () => {
    const paths = [
      `/api/v1/runs/example/private/${privateUID}`,
      `/api/v1/runs/example/agents/${privateUID}`,
      `/api/v1/runs/example/..%2Fprivate/${privateUID}`,
      `/api/v1/runs/example/private/${privateUID}/events`,
    ];
    for (const path of paths) {
      const response = await get(path);
      expect([NOT_FOUND, BAD_REQUEST], path).toContain(response.status);
      expect(await response.text(), path).not.toContain(PRIVATE_MARKER);
    }
  });

  it('shows only the own line of a run whose label a foreign pod copies', async () => {
    const r = await run('foreign-pod');
    const snap = await snapshot(r);
    expect(snap.attempts).toHaveLength(1);
    expect(JSON.stringify(snap)).not.toContain(FOREIGN_MARKER);
    const watched = await open(browser, r.link);
    const { page } = watched;
    await visible(assistantMessages(page).filter({ hasText: "The run's own line." }));
    await eventually(() => assistantMessages(page).count()).toBe(1);
    // close() checks that the foreign line is nowhere on the page.
    await close(watched);
  });
});
