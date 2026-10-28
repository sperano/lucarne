// Loading a run's snapshot and its events' source lines. The server and the
// UI ship in one binary and agree on the contract, which the server
// validates; the UI checks only the schema version, as it cannot run the
// schema validator (see validate.ts). An answer that is a redirect, or a
// 401, is the authenticating proxy asking to sign in again (see signin.ts).
import {
  API_PREFIX,
  SCHEMA_VERSION,
  type ErrorResponse,
  type RunKey,
  type Snapshot,
  type SourceLine,
} from './contract';

/** Why a request gave no document. */
export interface ApiFailure {
  readonly ok: false;
  readonly status: number;
  readonly code?: string;
  readonly message: string;
  /** The proxy in front of the server asks to sign in again. */
  readonly signIn?: true;
}

export type SnapshotLoad = { readonly ok: true; readonly snapshot: Snapshot } | ApiFailure;
export type SourceLoad = { readonly ok: true; readonly line: SourceLine } | ApiFailure;

const UNAUTHORIZED = 401;

/** The API path of a run's snapshot. */
export function snapshotPath(key: RunKey): string {
  return `${API_PREFIX}/runs/${key.cluster}/${key.namespace}/${key.taskUID}`;
}

/** The API path of the source line of a run's event. */
export function sourcePath(key: RunKey, eventId: string): string {
  return `${snapshotPath(key)}/source/${eventId}`;
}

/** Loads the snapshot of key; a network failure rejects, as fetch does. */
export async function loadSnapshot(
  key: RunKey,
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<SnapshotLoad> {
  const answer = await load(snapshotPath(key), fetcher, signal);
  if (!answer.ok) return answer;
  return isSnapshot(answer.body)
    ? { ok: true, snapshot: answer.body }
    : notA('snapshot', answer.status);
}

/**
 * Loads the source line of the event eventId of key's run, through the
 * same server and authorization as its snapshot; a network failure rejects.
 */
export async function loadSource(
  key: RunKey,
  eventId: string,
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<SourceLoad> {
  const answer = await load(sourcePath(key, eventId), fetcher, signal);
  if (!answer.ok) return answer;
  return isSourceLine(answer.body)
    ? { ok: true, line: answer.body }
    : notA('source line', answer.status);
}

/** A successful answer's body, which the caller narrows to its document, or why there is none. */
type Answer = { readonly ok: true; readonly status: number; readonly body: unknown } | ApiFailure;

async function load(path: string, fetcher: typeof fetch, signal?: AbortSignal): Promise<Answer> {
  const response = await fetcher(path, {
    headers: { Accept: 'application/json' },
    // The server never redirects an API request; a proxy redirecting to its sign-in page does.
    redirect: 'manual',
    ...(signal ? { signal } : {}),
  });
  if (response.type === 'opaqueredirect' || response.status === UNAUTHORIZED) {
    return { ok: false, status: response.status, message: 'sign-in required', signIn: true };
  }
  const body: unknown = await response.json().catch(() => undefined);
  if (response.ok) return { ok: true, status: response.status, body };
  if (isErrorResponse(body)) {
    return {
      ok: false,
      status: response.status,
      code: body.error.code,
      message: body.error.message,
    };
  }
  return { ok: false, status: response.status, message: response.statusText || 'request failed' };
}

function notA(what: string, status: number): ApiFailure {
  return { ok: false, status, message: `not a ${SCHEMA_VERSION} ${what}` };
}

function isSnapshot(body: unknown): body is Snapshot {
  return isVersioned(body);
}

function isSourceLine(body: unknown): body is SourceLine {
  return isVersioned(body);
}

function isVersioned(body: unknown): boolean {
  return (
    typeof body === 'object' && body !== null && 'schema' in body && body.schema === SCHEMA_VERSION
  );
}

function isErrorResponse(body: unknown): body is ErrorResponse {
  if (typeof body !== 'object' || body === null || !('error' in body)) return false;
  const { error } = body;
  return typeof error === 'object' && error !== null && 'code' in error && 'message' in error;
}
