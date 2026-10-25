import { describe, expect, it } from 'vitest';
import { parseRoute, runLink } from './route';

const UID = '3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b';
const RUN_PATH = `/runs/example/agents/${UID}`;
const ATTEMPT = '7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0';
const KEY = { cluster: 'example', namespace: 'agents', taskUID: UID };

describe('parseRoute', () => {
  it('opens the home page at the root', () => {
    expect(parseRoute('/', '', '')).toEqual({ page: 'home' });
  });

  it('opens a run in the conversation view by default', () => {
    expect(parseRoute(RUN_PATH, '', '')).toEqual({ page: 'run', key: KEY, view: 'conversation' });
  });

  it('keeps the attempt, view and event of a full link', () => {
    const route = parseRoute(
      RUN_PATH,
      `?attempt=${ATTEMPT}&view=inspector&utm=ignored`,
      `#event=${ATTEMPT}:42:1`,
    );
    expect(route).toEqual({
      page: 'run',
      key: KEY,
      view: 'inspector',
      attempt: ATTEMPT,
      event: `${ATTEMPT}:42:1`,
    });
  });

  it('accepts a gap event in the fragment', () => {
    expect(parseRoute(RUN_PATH, '', `#event=${ATTEMPT}:120:gap`)).toMatchObject({
      event: `${ATTEMPT}:120:gap`,
    });
  });

  it.each([
    ['a task name instead of a UID', '/runs/example/agents/example-claude-42', ''],
    ['an uppercase UID', `/runs/example/agents/${UID.toUpperCase()}`, ''],
    ['a trailing slash', `${RUN_PATH}/`, ''],
    ['a missing namespace', `/runs/example/${UID}`, ''],
    ['another page', '/settings', ''],
    ['a pod name as attempt', RUN_PATH, '?attempt=example-claude-42-x7k2p'],
    ['an unknown view', RUN_PATH, '?view=timeline'],
  ])('rejects %s', (_, path, search) => {
    expect(parseRoute(path, search, '')).toMatchObject({ page: 'invalid' });
  });

  it('rejects a malformed event fragment', () => {
    expect(parseRoute(RUN_PATH, '', '#event=42')).toMatchObject({ page: 'invalid' });
  });

  it('ignores fragments that are not event links', () => {
    expect(parseRoute(RUN_PATH, '', '#top')).toEqual({
      page: 'run',
      key: KEY,
      view: 'conversation',
    });
  });
});

describe('runLink', () => {
  it('is the inverse of parseRoute', () => {
    expect(runLink(KEY)).toBe(RUN_PATH);
  });
});
