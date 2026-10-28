// The runner fixtures' own lines, served as the source endpoint serves the
// lines of a run, for the inspector's tests. Not part of the app build.
import type { SourceLoad } from '../lib/api';
import { SCHEMA_VERSION, type RunnerKind } from '../lib/contract';
import { utf8Length } from '../lib/format';
import { isGapId, type LoadSource } from '../lib/sources';

const logs = import.meta.glob<string>('../../../fixtures/*/*/*.log', {
  eager: true,
  query: '?raw',
  import: 'default',
});

const LOG_PATH = /\/fixtures\/([a-z-]+)\/[^/]+\/([a-z-]+)\.log$/;
const NOT_FOUND = 404;

/** The lines of the fixture log of runner and scenario, or throws. */
export function fixtureLines(runner: RunnerKind, scenario: string): string[] {
  const found = Object.entries(logs).find(([path]) => {
    const match = LOG_PATH.exec(path);
    return match?.[1] === runner && match[2] === scenario;
  });
  if (!found) throw new Error(`no fixture log ${runner}/${scenario}`);
  return found[1].replace(/\n$/, '').split('\n');
}

/** Serves the lines as the source lines of the events of their run; a gap, or a line past them, is not held. */
export function linesSource(lines: readonly string[]): LoadSource & { readonly asked: string[] } {
  const asked: string[] = [];
  const load = (eventId: string): Promise<SourceLoad> => {
    asked.push(eventId);
    const [attempt = '', origin = '', line = ''] = eventId.split(':');
    const text = lines[Number(line) - 1];
    if (isGapId(eventId) || text === undefined || text === '') {
      return Promise.resolve({
        ok: false,
        status: NOT_FOUND,
        code: 'source_not_found',
        message: 'the run holds no source line for this event: a gap, or a line no longer held',
      });
    }
    return Promise.resolve({
      ok: true,
      line: {
        schema: SCHEMA_VERSION,
        attempt,
        origin,
        line: Number(line),
        text,
        bytes: utf8Length(text),
      },
    });
  };
  return Object.assign(load, { asked });
}

/** Serves the fixture log of runner and scenario. */
export function fixtureSource(
  runner: RunnerKind,
  scenario: string,
): ReturnType<typeof linesSource> {
  return linesSource(fixtureLines(runner, scenario));
}
