// The control API of lucarne-e2e (internal/e2e/cmd/lucarne-e2e/control.go),
// which drives the fake cluster, and the Claude Code lines a test writes to
// a live run. Every line is fabricated: the repository is public.
import { inject } from 'vitest';
import type { Snapshot } from '../src/lib/contract';

/** A run of the fake cluster, as the control API describes it. */
export interface RunInfo {
  readonly name: string;
  /** The run's deep link. */
  readonly link: string;
  /** The path of its snapshot. */
  readonly api: string;
}

/** How a finished Task ends (kubetest.PhaseSucceeded, kubetest.PhaseFailed). */
export type TaskPhase = 'Succeeded' | 'Failed';

const CONTROL_PREFIX = '/_e2e';

/** The base URL of the server, which globalSetup provides. */
export function baseURL(): string {
  return inject('baseURL');
}

async function call(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(`${baseURL()}${CONTROL_PREFIX}${path}`, init);
  if (!response.ok) {
    throw new Error(`${path}: ${String(response.status)} ${await response.text()}`);
  }
  return response;
}

async function post(path: string, body?: unknown): Promise<void> {
  await call(path, {
    method: 'POST',
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

/** Every run of the world, by name. */
export async function runs(): Promise<Record<string, RunInfo>> {
  return (await (await call('/runs')).json()) as Record<string, RunInfo>;
}

/** The run called name. */
export async function run(name: string): Promise<RunInfo> {
  const found = (await runs())[name];
  if (found === undefined) throw new Error(`no run ${name}`);
  return found;
}

/** A new running Claude Code run with an empty log. */
export async function live(): Promise<RunInfo> {
  return (await (await call('/live', { method: 'POST' })).json()) as RunInfo;
}

export const write = (r: RunInfo, lines: readonly string[]): Promise<void> =>
  post(`/runs/${r.name}/write`, { lines });
/** Breaks the server's log streams of the run: the source reads the log again. */
export const breakLog = (r: RunInfo): Promise<void> => post(`/runs/${r.name}/break`);
/** Makes the log start after the lines it holds now, as a rotation does. */
export const rotate = (r: RunInfo): Promise<void> => post(`/runs/${r.name}/rotate`);
export const finish = (r: RunInfo, exitCode: number, phase: TaskPhase): Promise<void> =>
  post(`/runs/${r.name}/finish?exit=${String(exitCode)}&phase=${phase}`);
/** Deletes the run's Task, as its TTL does. */
export const deleteTask = (r: RunInfo): Promise<void> => post(`/runs/${r.name}/delete`);
/** Ends the browser's SSE streams and holds their reconnects, as an outage does. */
export const cutStreams = (): Promise<void> => post('/streams/cut');
/** Lets the held reconnects through. */
export const restoreStreams = (): Promise<void> => post('/streams/restore');

/** Fetches a path of the server: an API answer, not through a browser. */
export function get(path: string): Promise<Response> {
  return fetch(`${baseURL()}${path}`);
}

/** The run's snapshot, as the API serves it. */
export async function snapshot(r: RunInfo): Promise<Snapshot> {
  const response = await get(r.api);
  if (!response.ok) throw new Error(`${r.api}: ${String(response.status)}`);
  return (await response.json()) as Snapshot;
}

// The fabricated Claude Code session of the generated lines, as in
// internal/e2e/scenario/runs.go.
const SESSION_ID = '0b1c2d3e-4f50-4617-8293-a4b5c6d7e8f9';
const MODEL = 'claude-opus-5-5';
const RUNNER_VERSION = '2.1.296';
const UUID_SERIAL_DIGITS = 12;

/** The fabricated UUID of the n-th message. */
function messageUUID(n: number): string {
  return `00000000-0000-4000-8000-${String(n).padStart(UUID_SERIAL_DIGITS, '0')}`;
}

/** The init line a Claude Code session starts with. */
export function claudeInit(): string {
  return JSON.stringify({
    type: 'system',
    subtype: 'init',
    cwd: '/workspace/repo',
    session_id: SESSION_ID,
    tools: ['Bash', 'Read'],
    model: MODEL,
    claude_code_version: RUNNER_VERSION,
    uuid: messageUUID(0),
  });
}

/** A Claude Code assistant message holding text, as the n-th message of the session. */
export function claudeText(n: number, text: string): string {
  return JSON.stringify({
    type: 'assistant',
    message: {
      model: MODEL,
      id: `msg_${String(n).padStart(UUID_SERIAL_DIGITS, '0')}`,
      type: 'message',
      role: 'assistant',
      content: [{ type: 'text', text }],
      stop_reason: null,
    },
    parent_tool_use_id: null,
    session_id: SESSION_ID,
    uuid: messageUUID(n),
  });
}

/** Messages first..first+count-1, each a distinct text made by text(n). */
export function claudeTexts(first: number, count: number, text: (n: number) => string): string[] {
  return Array.from({ length: count }, (_, i) => claudeText(first + i, text(first + i)));
}
