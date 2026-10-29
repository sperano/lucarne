import { describe, expect, it } from 'vitest';
import list from '../../../contract/v1/examples/RunList/two-namespaces.json';
import type { RunList } from './contract';
import { matchRuns } from './runsearch';

const { runs } = list as RunList;
const names = (query: string) => matchRuns(runs, query).map((run) => run.taskName);

describe('matchRuns', () => {
  it('keeps every run, in order, for a blank query', () => {
    expect(names('')).toEqual(runs.map((run) => run.taskName));
    expect(names('   ')).toHaveLength(runs.length);
  });

  it('matches part of the Task name, ignoring case', () => {
    expect(names('LOGIN')).toEqual(['fix-login-redirect']);
    expect(names('42')).toEqual(['review-pr-42']);
  });

  it('matches the namespace, the Task UID, the phase and the runner', () => {
    expect(names('review')).toEqual(['review-pr-42']);
    expect(names('9a0b1c2d')).toEqual(['review-pr-42']);
    expect(names('succeeded')).toEqual(['review-pr-42']);
    expect(names('claude code')).toEqual(['fix-login-redirect']);
    expect(names('codex')).toEqual(['review-pr-42']);
  });

  it('needs every word to match', () => {
    expect(names('agents running')).toEqual(['fix-login-redirect']);
    expect(names('agents codex')).toEqual([]);
  });

  it('matches nothing a run does not hold', () => {
    expect(names('no-such-task')).toEqual([]);
  });
});
