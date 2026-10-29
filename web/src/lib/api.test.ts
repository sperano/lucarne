import { describe, expect, it } from 'vitest';
import snapshot from '../../../contract/v1/examples/Snapshot/claude-session.json';
import notFound from '../../../contract/v1/examples/ErrorResponse/run-not-found.json';
import sourceNotFound from '../../../contract/v1/examples/ErrorResponse/source-not-found.json';
import sourceLine from '../../../contract/v1/examples/SourceLine/assistant-line.json';
import runList from '../../../contract/v1/examples/RunList/two-namespaces.json';
import { loadRuns, loadSnapshot, loadSource, RUNS_PATH, snapshotPath, sourcePath } from './api';

const KEY = snapshot.run.key;
const OK = 200;
const UNAUTHORIZED = 401;
const NOT_FOUND = 404;
const BAD_GATEWAY = 502;

function answering(status: number, body: unknown, statusText = ''): typeof fetch {
  return (input) => {
    expect(input).toBe(snapshotPath(KEY));
    const text = typeof body === 'string' ? body : JSON.stringify(body);
    return Promise.resolve(new Response(text, { status, statusText }));
  };
}

describe('loadSnapshot', () => {
  it('returns a snapshot of the contract version', async () => {
    expect(await loadSnapshot(KEY, answering(OK, snapshot))).toEqual({ ok: true, snapshot });
  });

  it('refuses another document', async () => {
    expect(await loadSnapshot(KEY, answering(OK, { schema: 'other/v2' }))).toMatchObject({
      ok: false,
      status: OK,
    });
  });

  it('returns the error of an ErrorResponse', async () => {
    expect(await loadSnapshot(KEY, answering(NOT_FOUND, notFound))).toEqual({
      ok: false,
      status: NOT_FOUND,
      code: notFound.error.code,
      message: notFound.error.message,
    });
  });

  it('returns the status of an answer that is not JSON', async () => {
    expect(await loadSnapshot(KEY, answering(BAD_GATEWAY, '<html>', 'Bad Gateway'))).toEqual({
      ok: false,
      status: BAD_GATEWAY,
      message: 'Bad Gateway',
    });
  });

  it('asks to sign in again on a redirect or a 401', async () => {
    const redirected = () => Promise.resolve({ type: 'opaqueredirect', status: 0 } as Response);
    expect(await loadSnapshot(KEY, redirected)).toMatchObject({ ok: false, signIn: true });
    expect(await loadSnapshot(KEY, answering(UNAUTHORIZED, ''))).toMatchObject({
      ok: false,
      status: UNAUTHORIZED,
      signIn: true,
    });
  });

  it('builds the API path of a run', () => {
    expect(snapshotPath(KEY)).toBe(
      '/api/v1/runs/example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b',
    );
  });
});

describe('loadSource', () => {
  const EVENT = snapshot.events[0]?.id ?? '';
  const answeringSource =
    (status: number, body: unknown): typeof fetch =>
    (input) => {
      expect(input).toBe(sourcePath(KEY, EVENT));
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    };

  it('builds the API path of an event source line', () => {
    expect(sourcePath(KEY, EVENT)).toBe(`${snapshotPath(KEY)}/source/${EVENT}`);
  });

  it('returns a source line of the contract version', async () => {
    expect(await loadSource(KEY, EVENT, answeringSource(OK, sourceLine))).toEqual({
      ok: true,
      line: sourceLine,
    });
    expect(await loadSource(KEY, EVENT, answeringSource(OK, { text: '{}' }))).toMatchObject({
      ok: false,
      status: OK,
    });
  });

  it('returns the error of a line the run does not hold', async () => {
    expect(await loadSource(KEY, EVENT, answeringSource(NOT_FOUND, sourceNotFound))).toEqual({
      ok: false,
      status: NOT_FOUND,
      code: 'source_not_found',
      message: sourceNotFound.error.message,
    });
  });

  it('asks to sign in again on a redirect', async () => {
    const redirected = () => Promise.resolve({ type: 'opaqueredirect', status: 0 } as Response);
    expect(await loadSource(KEY, EVENT, redirected)).toMatchObject({ ok: false, signIn: true });
  });
});

describe('loadRuns', () => {
  const at =
    (status: number, body: unknown): typeof fetch =>
    (input) => {
      expect(input).toBe(RUNS_PATH);
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    };

  it('returns a run list of the contract version', async () => {
    expect(await loadRuns(at(OK, runList))).toEqual({ ok: true, list: runList });
  });

  it('refuses another document', async () => {
    expect(await loadRuns(at(OK, { runs: [] }))).toMatchObject({ ok: false, status: OK });
  });

  it('returns the error of an ErrorResponse', async () => {
    expect(await loadRuns(at(NOT_FOUND, notFound))).toMatchObject({
      ok: false,
      status: NOT_FOUND,
      code: notFound.error.code,
    });
  });
});
