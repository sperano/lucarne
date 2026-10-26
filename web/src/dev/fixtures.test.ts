import { describe, expect, it } from 'vitest';
import { RUNNER_KINDS } from '../lib/contract';
import { schemaErrors } from '../lib/validate';
import { FIXTURE_RUNS } from './fixtures';

describe('fixture runs', () => {
  it('cover every runner', () => {
    const runners = new Set(FIXTURE_RUNS.map((f) => f.runner));
    expect([...runners].sort()).toEqual(RUNNER_KINDS.filter((k) => k !== 'unknown').sort());
  });

  it.each(FIXTURE_RUNS.map((f) => [`${f.runner}/${f.scenario}`, f] as const))(
    '%s is a valid snapshot',
    (_name, fixture) => {
      expect(schemaErrors('Snapshot', fixture.snapshot)).toEqual([]);
    },
  );
});
