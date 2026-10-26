// Loading a run's snapshot. The server and the UI ship in one binary and
// agree on the contract, which the server validates; the UI checks only
// the schema version, as it cannot run the schema validator (see
// validate.ts). An answer that is a redirect, or a 401, is the
// authenticating proxy asking to sign in again (see signin.ts).
import {
  API_PREFIX,
  SCHEMA_VERSION,
  type ErrorResponse,
  type RunKey,
  type Snapshot,
} from './contract';

export type SnapshotLoad =
  | { readonly ok: true; readonly snapshot: Snapshot }
  | {
      readonly ok: false;
      readonly status: number;
      readonly code?: string;
      readonly message: string;
      /** The proxy in front of the server asks to sign in again. */
      readonly signIn?: true;
    };

const UNAUTHORIZED = 401;

/** The API path of a run's snapshot. */
export function snapshotPath(key: RunKey): string {
  return `${API_PREFIX}/runs/${key.cluster}/${key.namespace}/${key.taskUID}`;
}

/** Loads the snapshot of key; a network failure rejects, as fetch does. */
export async function loadSnapshot(
  key: RunKey,
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<SnapshotLoad> {
  const response = await fetcher(snapshotPath(key), {
    headers: { Accept: 'application/json' },
    // The server never redirects an API request; a proxy redirecting to its sign-in page does.
    redirect: 'manual',
    ...(signal ? { signal } : {}),
  });
  if (response.type === 'opaqueredirect' || response.status === UNAUTHORIZED) {
    return { ok: false, status: response.status, message: 'sign-in required', signIn: true };
  }
  const body: unknown = await response.json().catch(() => undefined);
  if (response.ok) {
    if (isSnapshot(body)) return { ok: true, snapshot: body };
    return { ok: false, status: response.status, message: `not a ${SCHEMA_VERSION} snapshot` };
  }
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

function isSnapshot(body: unknown): body is Snapshot {
  return (
    typeof body === 'object' && body !== null && 'schema' in body && body.schema === SCHEMA_VERSION
  );
}

function isErrorResponse(body: unknown): body is ErrorResponse {
  if (typeof body !== 'object' || body === null || !('error' in body)) return false;
  const { error } = body;
  return typeof error === 'object' && error !== null && 'code' in error && 'message' in error;
}
