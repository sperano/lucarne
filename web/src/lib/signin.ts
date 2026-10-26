// Signing in again without losing the page's place. Lucarne sits behind an
// authenticating proxy: when the session expires, an API request is
// redirected to the sign-in page (fetch, with redirect: 'manual', sees an
// opaque redirect) or answered 401. Only the page itself can go through the
// sign-in, so the UI loads its own address again and the proxy sends it to
// sign in and back. The proxy keeps the path and query it was asked for, but
// a browser never sends the fragment (#event=) and a sign-in form loses it:
// the complete address is saved for this tab first and restored on return.

const DESTINATION_KEY = 'lucarne.destination';
/** A destination saved longer ago than this is stale: that sign-in was abandoned. */
export const DESTINATION_TTL_MS = 10 * 60 * 1000;
/** A page sent to sign in this recently is not sent again on its own, so a refusal cannot loop. */
export const SIGN_IN_INTERVAL_MS = 60 * 1000;

/** The parts of a location the destination is made of. */
export interface Place {
  readonly pathname: string;
  readonly search: string;
  readonly hash: string;
}

interface Destination {
  readonly href: string;
  readonly at: number;
  readonly restored: boolean;
}

/** The address of place within the site: path, query and fragment. */
export function hrefOf(place: Place): string {
  return `${place.pathname}${place.search}${place.hash}`;
}

/**
 * Sends the page to sign in, after saving its complete address, and
 * returns true. It returns false, doing nothing, when storage is unavailable
 * or the same address was sent to sign in less than SIGN_IN_INTERVAL_MS ago,
 * unless asked by the reader (force): the UI then offers a link instead.
 */
export function signIn(
  place: Place,
  storage: Storage | undefined,
  navigate: (href: string) => void,
  options: { readonly force?: boolean; readonly now?: number } = {},
): boolean {
  const now = options.now ?? Date.now();
  const href = hrefOf(place);
  const saved = read(storage);
  const recent = saved?.href === href && now - saved.at < SIGN_IN_INTERVAL_MS;
  if (options.force !== true && (storage === undefined || recent)) return false;
  write(storage, { href, at: now, restored: false });
  navigate(href);
  return true;
}

/**
 * The address to put back after a sign-in: the one saved for this path, when
 * it is recent and the proxy brought the page back with less of it (no
 * fragment, or neither query nor fragment). Undefined when nothing is to
 * restore. A destination is restored once.
 */
export function restoreDestination(
  place: Place,
  storage: Storage | undefined,
  now = Date.now(),
): string | undefined {
  const saved = read(storage);
  if (!saved || saved.restored || now - saved.at >= DESTINATION_TTL_MS) return undefined;
  const target = new URL(saved.href, 'http://lucarne.invalid');
  if (target.pathname !== place.pathname) return undefined;
  const searchKept = place.search === '' || place.search === target.search;
  const hashKept = place.hash === '' || place.hash === target.hash;
  if (!searchKept || !hashKept) return undefined;
  write(storage, { ...saved, restored: true });
  return saved.href === hrefOf(place) ? undefined : saved.href;
}

/** The tab's sessionStorage, or undefined where the browser refuses it. */
export function tabStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

function read(storage: Storage | undefined): Destination | undefined {
  try {
    const value: unknown = JSON.parse(storage?.getItem(DESTINATION_KEY) ?? 'null');
    return isDestination(value) ? value : undefined;
  } catch {
    return undefined;
  }
}

function write(storage: Storage | undefined, destination: Destination): void {
  try {
    storage?.setItem(DESTINATION_KEY, JSON.stringify(destination));
  } catch {
    // A full or refused storage loses only the fragment: the proxy keeps the rest.
  }
}

function isDestination(value: unknown): value is Destination {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.href === 'string' &&
    // Within the site only: '//host' would name another one.
    v.href.startsWith('/') &&
    !v.href.startsWith('//') &&
    typeof v.at === 'number' &&
    typeof v.restored === 'boolean'
  );
}
