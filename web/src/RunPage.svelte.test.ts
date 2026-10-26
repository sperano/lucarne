import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import notFound from '../../contract/v1/examples/ErrorResponse/run-not-found.json';
import type { Snapshot, TranscriptEvent } from './lib/contract';
import { parseRoute, runHref } from './lib/route';
import { restoreDestination } from './lib/signin';
import { ATTEMPT_ID, eventMaker } from './dev/events';
import { FIXTURE_KEY, FIXTURE_RUNS, findFixture, snapshotOf } from './dev/fixtures';
import { fakeServer, runningSnapshot, type Answer } from './dev/server';
import { FakeStreams } from './dev/stream';
import { render } from './render';
import RunPage from './RunPage.svelte';

const NOT_FOUND = 404;
const NOT_IMPLEMENTED = 501;

const PLACE = {
  pathname: runHref(FIXTURE_KEY),
  search: '',
  hash: `#event=${ATTEMPT_ID}:e3b0c44298fc1c14:2:0`,
};

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

function show(
  answers: readonly Answer[],
  link: { attempt?: string; event?: string; view?: 'conversation' | 'inspector' } = {},
) {
  const server = fakeServer(answers);
  const streams = new FakeStreams();
  const root = render(RunPage, {
    key: FIXTURE_KEY,
    view: link.view ?? 'conversation',
    attempt: link.attempt,
    event: link.event,
    fetcher: server.fetcher,
    openStream: streams.open,
    place: PLACE,
    navigate: (href: string) => visited.push(href),
  });
  return { root, streams, server };
}

const alert = (root: HTMLElement) => root.querySelector('[role=alert]')?.textContent ?? '';
const status = (root: HTMLElement) => root.querySelector('.stream')?.textContent ?? '';
// The text of each message shown, without its role line.
const messageTexts = (root: HTMLElement) =>
  [...root.querySelectorAll('.message')].map((m) =>
    [...m.children]
      .filter((c) => !c.classList.contains('role'))
      .map((c) => c.textContent.trim())
      .join(''),
  );

describe('RunPage loading', () => {
  it('loads the snapshot and shows the conversation', async () => {
    const { snapshot } = findFixture('claude-code', 'session-basic');
    const { root } = show([snapshot]);
    await expect.poll(() => root.querySelector('h2')?.textContent).toBe(snapshot.run.taskName);
  });

  it('says when the server does not serve transcripts yet', async () => {
    const body = { error: { code: 'not_implemented', message: 'not implemented' } };
    const { root } = show([{ status: NOT_IMPLEMENTED, body }]);
    await expect.poll(() => alert(root)).toContain('does not serve transcripts yet');
  });

  it('says when the run is not known', async () => {
    const { root } = show([{ status: NOT_FOUND, body: notFound }]);
    await expect.poll(() => alert(root)).toContain('not known');
  });

  it('does not build an inspector yet', () => {
    const { root, server } = show([], { view: 'inspector' });
    expect(root.querySelector('a')?.getAttribute('href')).toBe(runHref(FIXTURE_KEY));
    expect(server.requests()).toBe(0);
  });
});

describe('RunPage streaming', () => {
  // One fixture of each runner, a third loaded, the rest streamed.
  const runs = FIXTURE_RUNS.filter((f) => f.scenario === 'session-basic');

  it.each(runs.map((f) => [f.runner, f.snapshot] as const))(
    'streams a %s run',
    async (_r, done) => {
      const loaded = Math.floor(done.events.length / 3);
      const { root, streams } = show([runningSnapshot(done, loaded)]);
      await expect.poll(() => streams.opened.length).toBe(1);
      const stream = streams.last.open().live(loaded);
      for (const event of done.events.slice(loaded)) stream.event(event);
      let seq = done.events.length;
      for (const attempt of done.attempts) stream.attempt(++seq, attempt);
      stream.run(++seq, done.run).end(seq + 1);
      await expect.poll(() => status(root)).toContain('The run finished');
      expect(status(root)).toContain('Succeeded');
      for (const event of done.events) {
        if (event.kind === 'tool_call') expect(root.textContent).toContain(event.toolCall.name);
        if (event.kind === 'message' && event.message.type === 'text') {
          const first = event.message.text.replaceAll('**', '').split('\n')[0] ?? '';
          expect(root.textContent).toContain(first);
        }
      }
      expect(root.textContent).toContain('The log was read to the end');
      expect(stream.closed).toBe(true);
    },
  );

  it('reconnects without repeating a message', async () => {
    const make = eventMaker();
    const say = (text: string) =>
      make({ kind: 'message', message: { role: 'assistant', type: 'text', text } });
    const events: TranscriptEvent[] = ['one', 'two', 'three', 'four'].map(say);
    const all = snapshotOf('claude-code', 'example-run', events);
    // The second snapshot is the one a reload gets.
    const { root, streams, server } = show([runningSnapshot(all, 1), runningSnapshot(all, 4)]);
    await expect.poll(() => streams.opened.length).toBe(1);
    const [, two, three, four] = events;
    if (!two || !three || !four) throw new Error('no events');
    // The browser reconnects from the last event: the stream goes on.
    const stream = streams.last.open().live(1).event(two).fail(true);
    await expect.poll(() => status(root)).toContain('reconnecting');
    stream.open().event(three).live(3);
    await expect.poll(() => messageTexts(root)).toEqual(['one', 'two', 'three']);
    // A replay that repeats an update is not applied: the run is loaded again.
    stream.event(three);
    await expect.poll(() => streams.opened.length).toBe(2);
    expect(server.requests()).toBe(2);
    streams.last.open().live(4);
    await expect.poll(() => messageTexts(root)).toEqual(['one', 'two', 'three', 'four']);
    expect(stream.closed).toBe(true);
  });

  it('keeps the attempt the reader picked while another one streams', async () => {
    const second = '9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b.claude-code.0';
    const say = (attempt: string, text: string) =>
      eventMaker(attempt)({ kind: 'message', message: { role: 'assistant', type: 'text', text } });
    const first = snapshotOf('claude-code', 'example-run', [say(ATTEMPT_ID, 'first')]);
    const [attempt] = first.attempts;
    if (!attempt) throw new Error('no attempt');
    const retried = { ...attempt, id: second, podName: 'example-b' };
    const later = { ...say(second, 'second'), seq: 2 };
    const snapshot: Snapshot = {
      ...runningSnapshot({ ...first, attempts: [attempt, retried] }, 1),
      events: [first.events[0] as TranscriptEvent, later],
      stream: { ...first.stream, lastSeq: 2 },
    };
    const { root, streams } = show([snapshot]);
    await expect.poll(() => messageTexts(root)).toEqual(['second']);
    const select = root.querySelector('select');
    if (!select) throw new Error('no attempt picker');
    await userEvent.selectOptions(select, ATTEMPT_ID);
    expect(location.search).toBe(`?attempt=${ATTEMPT_ID}`);
    await expect.poll(() => messageTexts(root)).toEqual(['first']);
    streams.last
      .open()
      .live(2)
      .event({ ...say(second, 'more'), seq: 3, id: `${second}:x:9:0` });
    await expect.poll(() => root.querySelector('.items')?.textContent).toContain('first');
    expect(messageTexts(root)).toEqual(['first']);
  });
});

describe('RunPage incremental updates', () => {
  const make = eventMaker();
  const say = (text: string) =>
    make({ kind: 'message', message: { role: 'assistant', type: 'text', text } });
  const events = ['alpha needle', 'beta', 'gamma needle', 'delta needle', 'epsilon'].map(say);
  const all = snapshotOf('claude-code', 'example-run', events);
  const count = (root: HTMLElement) => root.querySelector('.finder .count')?.textContent;

  async function streaming() {
    const page = show([runningSnapshot(all, 3)]);
    await expect.poll(() => page.streams.opened.length).toBe(1);
    const stream = page.streams.last.open().live(3);
    return { ...page, stream };
  }

  it('keeps the search and the result shown', async () => {
    const { root, stream } = await streaming();
    const input = root.querySelector('input[type=search]');
    if (!input) throw new Error('no search field');
    await userEvent.fill(input, 'needle');
    await userEvent.keyboard('{Enter}{Enter}');
    expect(count(root)).toBe('2 of 2');
    for (const event of events.slice(3)) stream.event(event);
    await expect.poll(() => count(root)).toBe('2 of 3');
    expect((input as HTMLInputElement).value).toBe('needle');
    expect(root.querySelector('.items > li.current')?.textContent).toContain('gamma');
  });

  it('keeps the text the reader selected', async () => {
    const { root, stream } = await streaming();
    await expect.poll(() => messageTexts(root)).toHaveLength(3);
    const paragraph = root.querySelector('.message p:not(.role)');
    if (!paragraph) throw new Error('no paragraph');
    getSelection()?.selectAllChildren(paragraph);
    for (const event of events.slice(3)) stream.event(event);
    await expect.poll(() => messageTexts(root)).toHaveLength(5);
    expect(getSelection()?.toString()).toBe('alpha needle');
    expect(getSelection()?.anchorNode?.isConnected).toBe(true);
  });
});

describe('RunPage direct links', () => {
  const { snapshot } = findFixture('claude-code', 'session-basic');
  const event = snapshot.events.find((e) => e.kind === 'tool_result');
  if (!event) throw new Error('no tool result');
  const link = `${runHref(FIXTURE_KEY)}#event=${event.id}`;

  function open(answers: readonly Answer[]) {
    const url = new URL(link, location.origin);
    const route = parseRoute(url.pathname, url.search, url.hash);
    if (route.page !== 'run') throw new Error(`not a run link: ${link}`);
    return show(answers, { ...route });
  }

  it('opens the linked event again when the page is reloaded', async () => {
    for (let reload = 0; reload < 2; reload++) {
      const { root } = open([
        { ...snapshot, stream: { ...snapshot.stream, epoch: `e${String(reload)}` } },
      ]);
      await expect.poll(() => root.querySelector('.targeted')?.textContent).toContain('Read');
      expect(document.activeElement?.classList.contains('targeted')).toBe(true);
    }
  });

  it('says a run the UID no longer names is gone, never showing another', async () => {
    const reused = {
      ...snapshot,
      run: {
        ...snapshot.run,
        key: { ...FIXTURE_KEY, taskUID: '0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d' },
      },
    };
    const { root } = open([reused]);
    await expect.poll(() => alert(root)).toContain('another run');
    expect(root.querySelector('.transcript')).toBeNull();
    const gone = open([{ status: NOT_FOUND, body: notFound }]);
    await expect.poll(() => alert(gone.root)).toContain('not known');
  });

  it('says when the run goes away while it is shown', async () => {
    const { root, streams } = open([
      runningSnapshot(snapshot, 4),
      { status: NOT_FOUND, body: notFound },
    ]);
    await expect.poll(() => streams.opened.length).toBe(1);
    streams.last.open().live(4).fail(false);
    await expect.poll(() => status(root), { timeout: 3000 }).toContain('no longer on the server');
    expect(root.querySelector('.transcript')).not.toBeNull();
  });
});

describe('RunPage sign-in', () => {
  const complete = PLACE.pathname + PLACE.search + PLACE.hash;
  // Loaded without the fragment: an address differing only by one would only scroll.
  const loaded = PLACE.pathname + PLACE.search;
  const saved = () => restoreDestination({ ...PLACE, hash: '' }, sessionStorage);

  it('signs in again at once, keeping the complete address', async () => {
    const { root } = show(['redirect']);
    await expect.poll(() => visited).toEqual([loaded]);
    expect(saved()).toBe(complete);
    expect(alert(root)).toContain('Signing in again');
  });

  it('offers to sign in when the server asked just before', async () => {
    show(['redirect']);
    await expect.poll(() => visited).toHaveLength(1);
    const { root } = show(['redirect']);
    await expect.poll(() => alert(root)).toContain('did just before');
    expect(visited).toHaveLength(1);
    root.querySelector('button')?.click();
    expect(visited).toEqual([loaded, loaded]);
    expect(saved()).toBe(complete);
  });

  it('offers to sign in when the session expires while following', async () => {
    const { snapshot } = findFixture('codex', 'session-basic');
    const { root, streams } = show([runningSnapshot(snapshot, 2), 'redirect']);
    await expect.poll(() => streams.opened.length).toBe(1);
    streams.last.open().live(2).fail(false);
    await expect.poll(() => status(root), { timeout: 3000 }).toContain('session expired');
    expect(visited).toEqual([]);
    [...root.querySelectorAll('button')].find((b) => b.textContent === 'Sign in again')?.click();
    expect(visited).toEqual([loaded]);
    expect(saved()).toBe(complete);
  });
});
