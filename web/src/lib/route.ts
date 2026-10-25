// Deep links, as specified in contract/README.md:
//   /runs/{cluster}/{namespace}/{taskUID}[?attempt={attemptID}][&view={view}][#event={eventID}]
import schema from '../../../contract/v1/transcript.schema.json';
import type { RunKey } from './contract';

export const VIEWS = ['conversation', 'inspector'] as const;
export type View = (typeof VIEWS)[number];
export const DEFAULT_VIEW: View = 'conversation';

const RUNS_PREFIX = 'runs';
const RUN_PATH_SEGMENTS = 4;
const EVENT_FRAGMENT_PREFIX = '#event=';

// The identity patterns are the schema's own, so a link the UI accepts is
// one the API accepts.
const patterns = {
  dnsLabel: new RegExp(schema.$defs.DNSLabel.pattern),
  uid: new RegExp(schema.$defs.UID.pattern),
  attempt: new RegExp(schema.$defs.AttemptID.pattern),
  event: new RegExp(schema.$defs.EventID.pattern),
};

export type Route =
  | { page: 'home' }
  | { page: 'run'; key: RunKey; view: View; attempt?: string; event?: string }
  | { page: 'invalid'; reason: string };

/** Parses a location's path, query and fragment into a route. */
export function parseRoute(pathname: string, search: string, hash: string): Route {
  if (pathname === '/') return { page: 'home' };
  const segments = pathname.split('/').slice(1);
  if (segments[0] !== RUNS_PREFIX || segments.length !== RUN_PATH_SEGMENTS) {
    return { page: 'invalid', reason: `not a run link: ${pathname}` };
  }
  const [, cluster = '', namespace = '', taskUID = ''] = segments;
  if (!patterns.dnsLabel.test(cluster)) return invalid('cluster', cluster);
  if (!patterns.dnsLabel.test(namespace)) return invalid('namespace', namespace);
  if (!patterns.uid.test(taskUID)) return invalid('task UID', taskUID);

  const query = new URLSearchParams(search);
  const view = query.get('view') ?? DEFAULT_VIEW;
  if (!isView(view)) return invalid('view', view);
  const attempt = query.get('attempt');
  if (attempt !== null && !patterns.attempt.test(attempt)) return invalid('attempt', attempt);
  const event = eventFragment(hash);
  if (event !== null && !patterns.event.test(event)) return invalid('event', event);
  return {
    page: 'run',
    key: { cluster, namespace, taskUID },
    view,
    ...(attempt === null ? {} : { attempt }),
    ...(event === null ? {} : { event }),
  };
}

/** The canonical deep link of a run. */
export function runLink(key: RunKey): string {
  return `/${RUNS_PREFIX}/${key.cluster}/${key.namespace}/${key.taskUID}`;
}

/** The event ID of a `#event=` fragment, or null for any other fragment. */
function eventFragment(hash: string): string | null {
  return hash.startsWith(EVENT_FRAGMENT_PREFIX) ? hash.slice(EVENT_FRAGMENT_PREFIX.length) : null;
}

function isView(view: string): view is View {
  // includes on the literal tuple only accepts a View; widening it lets any string be tested.
  return (VIEWS as readonly string[]).includes(view);
}

function invalid(part: string, value: string): Route {
  return { page: 'invalid', reason: `invalid ${part}: ${value}` };
}
