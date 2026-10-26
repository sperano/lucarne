import { describe, expect, it } from 'vitest';
import notFound from '../../contract/v1/examples/ErrorResponse/run-not-found.json';
import { snapshotPath } from './lib/api';
import { runHref } from './lib/route';
import { FIXTURE_KEY, findFixture } from './dev/fixtures';
import { render } from './render';
import RunPage from './RunPage.svelte';

const OK = 200;
const NOT_FOUND = 404;
const NOT_IMPLEMENTED = 501;

function answering(status: number, body: unknown): typeof fetch {
  return (input) => {
    expect(input).toBe(snapshotPath(FIXTURE_KEY));
    return Promise.resolve(new Response(JSON.stringify(body), { status }));
  };
}

function show(fetcher: typeof fetch, view: 'conversation' | 'inspector' = 'conversation') {
  return render(RunPage, { key: FIXTURE_KEY, view, attempt: undefined, event: undefined, fetcher });
}

describe('RunPage', () => {
  it('loads the snapshot and shows the conversation', async () => {
    const { snapshot } = findFixture('claude-code', 'session-basic');
    const root = show(answering(OK, snapshot));
    await expect.poll(() => root.querySelector('h2')?.textContent).toBe(snapshot.run.taskName);
  });

  it('says when the server does not serve transcripts yet', async () => {
    const body = { error: { code: 'not_implemented', message: 'not implemented' } };
    const root = show(answering(NOT_IMPLEMENTED, body));
    await expect
      .poll(() => root.querySelector('[role=alert]')?.textContent)
      .toContain('does not serve transcripts yet');
  });

  it('says when the run is not known', async () => {
    const root = show(answering(NOT_FOUND, notFound));
    await expect.poll(() => root.querySelector('[role=alert]')?.textContent).toContain('not known');
  });

  it('says when the server cannot be reached', async () => {
    const root = show(() => Promise.reject(new TypeError('Failed to fetch')));
    await expect
      .poll(() => root.querySelector('[role=alert]')?.textContent)
      .toContain('Failed to fetch');
  });

  it('does not build an inspector yet', () => {
    const root = show(() => Promise.reject(new Error('the inspector loads nothing')), 'inspector');
    expect(root.querySelector('a')?.getAttribute('href')).toBe(runHref(FIXTURE_KEY));
  });
});
