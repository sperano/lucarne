import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import example from '../../contract/v1/examples/RunList/two-namespaces.json';
import { RUNS_PATH } from './lib/api';
import type { Run, RunList } from './lib/contract';
import { runLink } from './lib/route';
import { SHOWN_RUNS } from './lib/runsearch';
import { render } from './render';
import RunSearch from './RunSearch.svelte';

const OK = 200;
const NOT_IMPLEMENTED = 501;
const UNAVAILABLE = 503;

const LIST = example as RunList;
const PLACE = { pathname: '/', search: '', hash: '' };

type Answer = RunList | { readonly status: number; readonly body: unknown } | 'redirect';

let visited: string[];
let address: string;

beforeEach(() => {
  visited = [];
  address = location.href;
  sessionStorage.clear();
});

afterEach(() => {
  history.replaceState(null, '', address);
  sessionStorage.clear();
});

/** Mounts the search over a server answering the run list with answers, the last one again once they run out. */
function show(answers: readonly Answer[], query = '') {
  let requests = 0;
  const fetcher: typeof fetch = (input) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url !== RUNS_PATH) throw new Error(`unexpected request ${url}`);
    const answer = answers[Math.min(requests++, answers.length - 1)];
    if (answer === undefined) throw new Error('no answer');
    if (answer === 'redirect') {
      // What fetch gives for a redirect with redirect: 'manual'; Response cannot make one.
      return Promise.resolve({ type: 'opaqueredirect', status: 0, ok: false } as Response);
    }
    const [status, body] = 'status' in answer ? [answer.status, answer.body] : [OK, answer];
    return Promise.resolve(new Response(JSON.stringify(body), { status }));
  };
  const root = render(RunSearch, {
    query,
    fetcher,
    place: PLACE,
    navigate: (href: string) => visited.push(href),
  });
  return { root, requests: () => requests };
}

const links = (root: HTMLElement) => [...root.querySelectorAll<HTMLAnchorElement>('ol a')];
const names = (root: HTMLElement) =>
  links(root).map((a) => a.querySelector('.name')?.textContent ?? '');
const count = (root: HTMLElement) => root.querySelector('.count')?.textContent.trim() ?? '';
const alert = (root: HTMLElement) => root.querySelector('[role=alert]')?.textContent ?? '';
const box = (root: HTMLElement) => {
  const input = root.querySelector<HTMLInputElement>('input[type=search]');
  if (!input) throw new Error('no search box');
  return input;
};

function manyRuns(n: number): Run[] {
  const [first] = LIST.runs;
  if (!first) throw new Error('no run in the example');
  return Array.from({ length: n }, (_, i) => ({
    ...first,
    key: { ...first.key, taskUID: `00000000-0000-4000-8000-${String(i).padStart(12, '0')}` },
    taskName: `task-${String(i)}`,
  }));
}

describe('RunSearch', () => {
  it('lists the runs in the server order, each a link to its run', async () => {
    const { root } = show([LIST]);
    await expect.poll(() => names(root)).toEqual(LIST.runs.map((run) => run.taskName));
    expect(links(root).map((a) => a.getAttribute('href'))).toEqual(
      LIST.runs.map((run) => runLink(run.key)),
    );
    expect(count(root)).toBe('3 runs, newest first.');
    expect(document.activeElement).toBe(box(root));
  });

  it('narrows the list as the reader types, and keeps the query in the address', async () => {
    const { root } = show([LIST]);
    await expect.poll(() => names(root)).toHaveLength(LIST.runs.length);
    await userEvent.fill(box(root), 'agents RUNNING');
    expect(names(root)).toEqual(['fix-login-redirect']);
    expect(count(root)).toBe('1 of 3 runs match, newest first.');
    expect(new URLSearchParams(location.search).get('q')).toBe('agents RUNNING');
    await userEvent.fill(box(root), 'nothing-like-it');
    expect(names(root)).toEqual([]);
    expect(count(root)).toBe('No run matches “nothing-like-it”.');
    await userEvent.fill(box(root), '');
    expect(location.pathname + location.search).toBe('/');
  });

  it('starts from the query of its address', async () => {
    const { root } = show([LIST], 'codex');
    await expect.poll(() => names(root)).toEqual(['review-pr-42']);
    expect(box(root).value).toBe('codex');
  });

  it('opens the first match on Enter', async () => {
    const { root } = show([LIST]);
    await expect.poll(() => names(root)).toHaveLength(LIST.runs.length);
    await userEvent.fill(box(root), 'review');
    await userEvent.keyboard('{Enter}');
    const review = LIST.runs.find((run) => run.taskName === 'review-pr-42');
    if (!review) throw new Error('no review run in the example');
    expect(visited).toEqual([runLink(review.key)]);
  });

  it('moves between the matches with the arrow keys', async () => {
    const { root } = show([LIST]);
    await expect.poll(() => names(root)).toHaveLength(LIST.runs.length);
    const [first, second] = links(root);
    await userEvent.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(first);
    await userEvent.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(second);
    await userEvent.keyboard('{ArrowUp}{ArrowUp}');
    expect(document.activeElement).toBe(box(root));
  });

  it('shows a window of the matches, and says how many it leaves out', async () => {
    const extra = 5;
    const list: RunList = { ...LIST, runs: manyRuns(SHOWN_RUNS + extra), omittedRuns: 12 };
    const { root } = show([list]);
    await expect.poll(() => links(root)).toHaveLength(SHOWN_RUNS);
    const notes = [...root.querySelectorAll('[role=note]')].map((n) => n.textContent);
    expect(notes.join(' ')).toContain('the 12 older ones are left out');
    expect(notes.join(' ')).toContain(`reach the ${String(extra)} others`);
    await userEvent.fill(box(root), `task-${String(SHOWN_RUNS + extra - 1)}`);
    expect(names(root)).toEqual([`task-${String(SHOWN_RUNS + extra - 1)}`]);
  });

  it('says when there is no run at all', async () => {
    const { root } = show([{ ...LIST, runs: [] }]);
    await expect.poll(() => count(root)).toContain('No run to list');
  });

  it('reloads the list on demand', async () => {
    const { root, requests } = show([{ ...LIST, runs: [] }, LIST]);
    await expect.poll(() => count(root)).toContain('No run to list');
    const reload = [...root.querySelectorAll('button')].find(
      (b) => b.textContent.trim() === 'Reload',
    );
    if (!reload) throw new Error('no reload button');
    await userEvent.click(reload);
    await expect.poll(() => names(root)).toHaveLength(LIST.runs.length);
    expect(requests()).toBe(2);
  });
});

describe('RunSearch failures', () => {
  it('says when the server reads no cluster', async () => {
    const body = { error: { code: 'not_implemented', message: 'no cluster' } };
    const { root } = show([{ status: NOT_IMPLEMENTED, body }]);
    await expect.poll(() => alert(root)).toContain('reads no cluster');
  });

  it("names the server's error", async () => {
    const body = { error: { code: 'unavailable', message: 'the Kubernetes API did not answer' } };
    const { root } = show([{ status: UNAVAILABLE, body }]);
    await expect
      .poll(() => alert(root))
      .toContain('(503 unavailable): the Kubernetes API did not answer');
  });

  it('goes to sign in again when the proxy asks', async () => {
    show(['redirect']);
    await expect.poll(() => visited).toEqual(['/']);
  });
});
