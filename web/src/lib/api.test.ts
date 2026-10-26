import { describe, expect, it } from 'vitest';
import snapshot from '../../../contract/v1/examples/Snapshot/claude-session.json';
import notFound from '../../../contract/v1/examples/ErrorResponse/run-not-found.json';
import { loadSnapshot, snapshotPath } from './api';

const KEY = snapshot.run.key;
const OK = 200;
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

  it('builds the API path of a run', () => {
    expect(snapshotPath(KEY)).toBe(
      '/api/v1/runs/example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b',
    );
  });
});
