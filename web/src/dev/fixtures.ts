// The runner fixtures as the adapters normalize them (their golden files),
// wrapped in fabricated snapshots: what the conversation's tests and the
// development preview (fixtures.html) render. Not part of the app build.
import contractInfo from '../../../contract/v1/examples/ContractInfo/v1.json';
import {
  SCHEMA_VERSION,
  type Attempt,
  type Limits,
  type RunnerKind,
  type Snapshot,
  type TranscriptEvent,
} from '../lib/contract';

const goldens = import.meta.glob<TranscriptEvent[]>(
  '../../../internal/adapter/*/testdata/*.golden.json',
  { eager: true, import: 'default' },
);

/** The runner of each adapter package. */
const ADAPTER_RUNNERS: Readonly<Record<string, RunnerKind>> = {
  claude: 'claude-code',
  codex: 'codex',
  opencode: 'opencode',
};

const GOLDEN_PATH = /\/internal\/adapter\/([a-z]+)\/testdata\/([a-z-]+)\.golden\.json$/;

export const LIMITS: Limits = contractInfo.limits;

export const FIXTURE_KEY = {
  cluster: 'example',
  namespace: 'agents',
  taskUID: '3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b',
} as const;

export interface FixtureRun {
  readonly runner: RunnerKind;
  readonly scenario: string;
  readonly snapshot: Snapshot;
}

/** Every golden file as a run, by runner then scenario. */
export const FIXTURE_RUNS: readonly FixtureRun[] = Object.entries(goldens)
  .map(([path, events]) => fixtureRun(path, events))
  .sort((a, b) => `${a.runner}/${a.scenario}`.localeCompare(`${b.runner}/${b.scenario}`));

/** The fixture run of a golden file, named by its path. */
export function fixtureRun(path: string, events: readonly TranscriptEvent[]): FixtureRun {
  const match = GOLDEN_PATH.exec(path);
  const runner = match ? ADAPTER_RUNNERS[match[1] ?? ''] : undefined;
  const scenario = match?.[2];
  if (runner === undefined || scenario === undefined) throw new Error(`not a golden file: ${path}`);
  return { runner, scenario, snapshot: snapshotOf(runner, `example-${scenario}`, events) };
}

/** The fixture run of runner and scenario, or throws. */
export function findFixture(runner: RunnerKind, scenario: string): FixtureRun {
  const found = FIXTURE_RUNS.find((f) => f.runner === runner && f.scenario === scenario);
  if (!found) throw new Error(`no fixture ${runner}/${scenario}`);
  return found;
}

/** A fabricated snapshot of one ended attempt holding events. */
export function snapshotOf(
  runner: RunnerKind,
  taskName: string,
  events: readonly TranscriptEvent[],
): Snapshot {
  const attemptId = events[0]?.attempt ?? `7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.${runner}.0`;
  return {
    schema: SCHEMA_VERSION,
    run: { key: FIXTURE_KEY, taskName, runner, phase: 'succeeded' },
    attempts: [attemptOf(attemptId, runner, events)],
    events,
    stream: { epoch: 'fixture', lastSeq: events.at(-1)?.seq ?? 0, droppedEvents: 0 },
    limits: LIMITS,
  };
}

function attemptOf(id: string, runner: RunnerKind, events: readonly TranscriptEvent[]): Attempt {
  const [podUID = '', container = '', restart = '0'] = id.split('.');
  return {
    id,
    key: { podUID, container, restart: Number(restart) },
    podName: `example-${runner}-x7k2p`,
    runner: { kind: runner },
    container: { state: 'terminated', exitCode: 0, reason: 'Completed' },
    coverage: {
      state: 'ended',
      lines: Math.max(0, ...events.map((e) => e.source.line)),
      gaps: events.filter((e) => e.kind === 'gap').length,
      truncatedLines: 0,
    },
  };
}
