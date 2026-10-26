import { describe, expect, it } from 'vitest';
import notFound from '../../../contract/v1/examples/ErrorResponse/run-not-found.json';
import unavailable from '../../../contract/v1/examples/ErrorResponse/source-unavailable.json';
import type { Snapshot, TranscriptEvent } from './contract';
import { ATTEMPT_ID, eventMaker } from '../dev/events';
import { FIXTURE_KEY, snapshotOf } from '../dev/fixtures';
import { FakeStreams } from '../dev/stream';
import { fakeServer, type Answer } from '../dev/server';
import { LiveRun, RETRY_BASE_MS, type LivePage } from './live';

const NOT_FOUND = 404;
const UNAVAILABLE = 503;

/** A run followed with fakes: answers are served in order, the last one again. */
function follow(...answers: Answer[]) {
  const streams = new FakeStreams();
  const pages: LivePage[] = [];
  const timers: { ms: number; run: () => void }[] = [];
  const { fetcher, requests } = fakeServer(answers);
  const run = new LiveRun({
    key: FIXTURE_KEY,
    fetcher,
    openStream: streams.open,
    onchange: (page) => pages.push(page),
    schedule: (flush) => {
      flush();
    },
    timer: (retry, ms) => {
      timers.push({ ms, run: retry });
      return () => {};
    },
  });
  run.start();
  const page = () => pages.at(-1);
  const ready = () => {
    const p = page();
    if (p?.phase !== 'ready') throw new Error(`not ready: ${JSON.stringify(p)}`);
    return p;
  };
  const texts = () =>
    ready()
      .view.conversationOf(ATTEMPT_ID)
      .items.map((i) =>
        i.kind === 'event' && i.event.kind === 'message' ? i.event.message.text : '',
      );
  return { run, streams, page, ready, texts, timers, requests };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

function messages(count: number, from = 0): TranscriptEvent[] {
  const make = eventMaker();
  const all = Array.from({ length: from + count }, (_, i) =>
    make({ kind: 'message', message: { role: 'assistant', type: 'text', text: `m${String(i)}` } }),
  );
  return all.slice(from);
}

function snapshot(events: readonly TranscriptEvent[], epoch = 'fixture'): Snapshot {
  const base = snapshotOf('claude-code', 'example-run', events);
  return { ...base, stream: { ...base.stream, epoch } };
}

describe('LiveRun', () => {
  it('loads the snapshot, then follows the stream after it until the end', async () => {
    const live = follow(snapshot(messages(2)));
    expect(live.page()).toBeUndefined();
    await settle();
    expect(live.streams.last.after).toBe('fixture:2');
    expect(live.ready().connection.state).toBe('connecting');
    const stream = live.streams.last.open();
    expect(live.ready().connection.state).toBe('catching_up');
    const [third] = messages(1, 2);
    if (!third) throw new Error('no event');
    stream.event(third).live(3);
    expect(live.ready().connection.state).toBe('live');
    expect(live.texts()).toEqual(['m0', 'm1', 'm2']);
    stream.end(4);
    expect(live.ready().connection).toEqual({ state: 'ended', reason: 'run_finished' });
    expect(stream.closed).toBe(true);
  });

  it('resumes after a reconnect of the browser without repeating anything', async () => {
    const live = follow(snapshot(messages(1)));
    await settle();
    const [second, third] = messages(2, 1);
    if (!second || !third) throw new Error('no event');
    const stream = live.streams.last.open().live(1).event(second);
    stream.fail(true);
    expect(live.ready().connection.state).toBe('reconnecting');
    stream.open().event(third).live(3);
    expect(live.texts()).toEqual(['m0', 'm1', 'm2']);
    expect(live.streams.opened).toHaveLength(1);
    expect(live.requests()).toBe(1);
  });

  it('reloads the snapshot when an update is out of order, so nothing shows twice', async () => {
    const all = messages(3);
    const live = follow(snapshot(all.slice(0, 2)), snapshot(all));
    await settle();
    const [repeated] = all.slice(1, 2);
    if (!repeated) throw new Error('no event');
    live.streams.last.open().event(repeated);
    expect(live.ready().connection).toEqual({ state: 'reloading', reason: 'out_of_order' });
    expect(live.streams.opened[0]?.closed).toBe(true);
    await settle();
    expect(live.texts()).toEqual(['m0', 'm1', 'm2']);
    expect(live.streams.last.after).toBe('fixture:3');
  });

  it('starts over on a reset', async () => {
    const all = messages(2);
    const live = follow(snapshot(all.slice(0, 1)), snapshot(all, 'restarted'));
    await settle();
    live.streams.last.open().reset('epoch_changed', 'restarted');
    await settle();
    expect(live.texts()).toEqual(['m0', 'm1']);
    expect(live.ready().view.stream.epoch).toBe('restarted');
    expect(live.streams.last.after).toBe('restarted:2');
  });

  it('reopens a stream the server closed, keeping what it holds when the epoch is the same', async () => {
    const all = messages(2);
    const live = follow(snapshot(all.slice(0, 1)), snapshot(all));
    await settle();
    const [second] = all.slice(1);
    if (!second) throw new Error('no event');
    live.streams.last.open().live(1).event(second).fail(false);
    expect(live.ready().connection.state).toBe('reconnecting');
    expect(live.timers.map((t) => t.ms)).toEqual([RETRY_BASE_MS]);
    live.timers[0]?.run();
    await settle();
    // The snapshot has m1 too, but the state held already does: the stream resumes after it.
    expect(live.texts()).toEqual(['m0', 'm1']);
    expect(live.streams.last.after).toBe('fixture:2');
  });

  it('says a run the server no longer has is gone, keeping what was loaded', async () => {
    const live = follow(snapshot(messages(1)), { status: NOT_FOUND, body: notFound });
    await settle();
    live.streams.last.open().fail(false);
    live.timers[0]?.run();
    await settle();
    expect(live.ready().connection).toMatchObject({
      state: 'stopped',
      failure: { status: NOT_FOUND, code: 'run_not_found' },
    });
    expect(live.texts()).toEqual(['m0']);
    expect(live.timers).toHaveLength(1);
  });

  it('retries an unavailable server, waiting longer each time', async () => {
    const answer = { status: UNAVAILABLE, body: unavailable };
    const live = follow(answer, answer, snapshot(messages(1)));
    await settle();
    expect(live.page()).toMatchObject({ phase: 'failed', retryInMs: RETRY_BASE_MS });
    live.timers[0]?.run();
    await settle();
    expect(live.page()).toMatchObject({ phase: 'failed', retryInMs: 2 * RETRY_BASE_MS });
    live.timers[1]?.run();
    await settle();
    expect(live.texts()).toEqual(['m0']);
  });

  it('never shows another run than the link names', async () => {
    const other = snapshot(messages(1));
    const live = follow({
      ...other,
      run: {
        ...other.run,
        key: { ...FIXTURE_KEY, taskUID: '0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d' },
      },
    });
    await settle();
    expect(live.page()).toMatchObject({ phase: 'failed', failure: { code: 'wrong_run' } });
    expect(live.streams.opened).toHaveLength(0);
  });

  it('stops for a sign-in, without retrying', async () => {
    const live = follow('redirect');
    await settle();
    expect(live.page()).toMatchObject({ phase: 'failed', failure: { signIn: true } });
    expect(live.timers).toHaveLength(0);
  });

  it('ignores the stream once stopped', async () => {
    const live = follow(snapshot(messages(1)));
    await settle();
    const stream = live.streams.last.open();
    const pages = live.page();
    live.run.stop();
    expect(stream.closed).toBe(true);
    expect(live.page()).toBe(pages);
  });
});
