// A scripted stand-in for the server's snapshot endpoint, for tests: it
// answers each request with the next answer, the last one again once they
// run out, and records what was asked.
import { snapshotPath } from '../lib/api';
import type { RunKey, Snapshot, TranscriptEvent } from '../lib/contract';
import { FIXTURE_KEY } from './fixtures';

const OK = 200;

/** A snapshot, an error status with its body, or the redirect of a proxy asking to sign in. */
export type Answer = Snapshot | { readonly status: number; readonly body: unknown } | 'redirect';

export interface FakeServer {
  readonly fetcher: typeof fetch;
  /** How many snapshots were asked for. */
  readonly requests: () => number;
}

/** Serves answers to the snapshot requests of key's run, refusing any other path. */
export function fakeServer(answers: readonly Answer[], key: RunKey = FIXTURE_KEY): FakeServer {
  let requests = 0;
  const fetcher: typeof fetch = (input) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (url !== snapshotPath(key)) throw new Error(`unexpected request ${url}`);
    const answer = answers[Math.min(requests++, answers.length - 1)];
    if (answer === undefined) throw new Error('no answer');
    if (answer === 'redirect') {
      // What fetch gives for a redirect with redirect: 'manual'; Response cannot make one.
      return Promise.resolve({ type: 'opaqueredirect', status: 0, ok: false } as Response);
    }
    const [status, body] = 'status' in answer ? [answer.status, answer.body] : [OK, answer];
    return Promise.resolve(new Response(JSON.stringify(body), { status }));
  };
  return { fetcher, requests: () => requests };
}

/**
 * The snapshot of a run still running: snapshot's run and attempts, the
 * first count of its events, the log being followed.
 */
export function runningSnapshot(snapshot: Snapshot, count: number): Snapshot {
  const events: TranscriptEvent[] = snapshot.events.slice(0, count);
  return {
    ...snapshot,
    run: { ...snapshot.run, phase: 'running' },
    attempts: snapshot.attempts.map((a) => ({
      ...a,
      container: { state: 'running' },
      coverage: { ...a.coverage, state: 'following' },
    })),
    events,
    stream: { ...snapshot.stream, lastSeq: events.at(-1)?.seq ?? 0 },
  };
}
