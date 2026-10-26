// Following a run live: load its snapshot, open its stream after the
// snapshot's cursor, apply each update in seq order, and say at every moment
// how the connection stands (contract/README.md, "SSE stream and replay").
//
// - A reconnect of the browser's EventSource resumes from Last-Event-ID, the
//   last update applied, so it neither repeats nor skips one.
// - A reset, an update out of order or a malformed one discards the state:
//   the snapshot is loaded again and the stream reopened from it.
// - A stream the server closed without an end (an error status) is retried
//   after loading the snapshot again, which says why: a run that is gone is
//   said to be gone, and what was loaded stays shown, marked as such.
// - Changes reach the page at most once per frame (schedule), however fast
//   a replay arrives.
import { loadSnapshot, type SnapshotLoad } from './api';
import type { EndReason, ResetReason, RunKey, Snapshot } from './contract';
import { RunStore, type RunView } from './runstore';
import { STREAM_EVENT_NAMES, decodeMessage, eventsPath, type StreamMessage } from './updates';

/** EventSource.readyState while the browser reconnects. */
const CONNECTING = 0;
/** EventSource.readyState once it gave up, or was closed. */
const CLOSED = 2;

/** The first wait before trying again after a failure; it doubles up to RETRY_MAX_MS. */
export const RETRY_BASE_MS = 1000;
export const RETRY_MAX_MS = 30_000;

const SERVER_ERROR = 500;
const NOT_IMPLEMENTED = 501;

/** What the controller needs of an EventSource. */
export interface StreamSource {
  readonly readyState: number;
  addEventListener(type: string, listener: (event: Event) => void): void;
  close(): void;
}

export type OpenStream = (url: string) => StreamSource;

/** Why a snapshot could not be had. */
export interface LoadFailure {
  readonly status?: number;
  readonly code?: string;
  readonly message: string;
  readonly signIn?: true;
}

export type Connection =
  | { readonly state: 'connecting' }
  /** The stream replays updates that happened before it opened. */
  | { readonly state: 'catching_up' }
  | { readonly state: 'live' }
  /** The browser lost the stream and reconnects from the last update. */
  | { readonly state: 'reconnecting' }
  /** The state is discarded and the snapshot loaded again. */
  | { readonly state: 'reloading'; readonly reason: ReloadReason }
  /** Loading failed; the next try is in retryInMs. */
  | { readonly state: 'retrying'; readonly failure: LoadFailure; readonly retryInMs: number }
  /** Nothing more will come: the run ended. */
  | { readonly state: 'ended'; readonly reason: EndReason }
  /** The server no longer has the run, or answers for it in a way retrying cannot change. */
  | { readonly state: 'stopped'; readonly failure: LoadFailure };

export type ReloadReason = ResetReason | 'out_of_order' | 'malformed';

export type LivePage =
  | { readonly phase: 'loading' }
  /** Nothing could be loaded; retryInMs when it will be tried again. */
  | { readonly phase: 'failed'; readonly failure: LoadFailure; readonly retryInMs?: number }
  | { readonly phase: 'ready'; readonly view: RunView; readonly connection: Connection };

export interface LiveOptions {
  readonly key: RunKey;
  readonly onchange: (page: LivePage) => void;
  readonly fetcher?: typeof fetch;
  readonly openStream?: OpenStream;
  /** Runs flush before the next frame; requestAnimationFrame by default. */
  readonly schedule?: (flush: () => void) => void;
  /** Runs retry after ms and returns a cancel; setTimeout by default. */
  readonly timer?: (retry: () => void, ms: number) => () => void;
}

/** Follows one run until stopped. */
export class LiveRun {
  private readonly options: LiveOptions;
  private store: RunStore | undefined;
  private connection: Connection = { state: 'connecting' };
  private failure: { failure: LoadFailure; retryInMs?: number } | undefined;
  private source: StreamSource | undefined;
  private loading: AbortController | undefined;
  private cancelRetry: (() => void) | undefined;
  private failures = 0;
  /** A reset or a lost update: the next snapshot replaces the state whatever its epoch. */
  private discard = false;
  private scheduled = false;
  private stopped = false;

  constructor(options: LiveOptions) {
    this.options = options;
  }

  start(): void {
    void this.load();
  }

  stop(): void {
    this.stopped = true;
    this.closeStream();
    this.loading?.abort();
    this.cancelRetry?.();
  }

  /** Loads the snapshot, then opens the stream after it. */
  private async load(): Promise<void> {
    this.closeStream();
    this.loading?.abort();
    const controller = new AbortController();
    this.loading = controller;
    let result: SnapshotLoad;
    try {
      result = await loadSnapshot(
        this.options.key,
        this.options.fetcher ?? fetch,
        controller.signal,
      );
    } catch (err) {
      if (controller.signal.aborted || this.stopped) return;
      this.failed({ message: err instanceof Error ? err.message : String(err) });
      return;
    }
    if (controller.signal.aborted || this.stopped) return;
    if (result.ok) this.loaded(result.snapshot);
    else this.failed(failureOf(result));
  }

  private loaded(snapshot: Snapshot): void {
    const { key } = this.options;
    const got = snapshot.run.key;
    if (
      got.cluster !== key.cluster ||
      got.namespace !== key.namespace ||
      got.taskUID !== key.taskUID
    ) {
      // Never show another run under this link, whatever the server answers.
      this.halt({ code: 'wrong_run', message: 'the server answered with another run' });
      return;
    }
    // The same epoch is the same history: the stream resumes from what is held.
    if (this.discard || this.store?.epoch !== snapshot.stream.epoch) {
      this.store = new RunStore(snapshot);
    }
    this.discard = false;
    this.failure = undefined;
    this.openStream(this.store.cursor);
    this.notify();
  }

  private failed(failure: LoadFailure): void {
    if (!retryable(failure)) {
      this.halt(failure);
      return;
    }
    const retryInMs = Math.min(RETRY_MAX_MS, RETRY_BASE_MS * 2 ** this.failures);
    this.failures++;
    if (this.store) this.connection = { state: 'retrying', failure, retryInMs };
    else this.failure = { failure, retryInMs };
    this.retryIn(retryInMs);
    this.notify();
  }

  /** Stops on a failure no retry can change. */
  private halt(failure: LoadFailure): void {
    this.closeStream();
    if (this.store) this.connection = { state: 'stopped', failure };
    else this.failure = { failure };
    this.notify();
  }

  private retryIn(ms: number): void {
    this.cancelRetry?.();
    const timer =
      this.options.timer ??
      ((retry, delay) => {
        const id = setTimeout(retry, delay);
        return () => {
          clearTimeout(id);
        };
      });
    this.cancelRetry = timer(() => {
      this.cancelRetry = undefined;
      if (!this.stopped) void this.load();
    }, ms);
  }

  private openStream(cursor: string): void {
    const open = this.options.openStream ?? ((url) => new EventSource(url));
    const source = open(eventsPath(this.options.key, cursor));
    this.source = source;
    this.connection = { state: 'connecting' };
    const ifCurrent = (handle: (event: Event) => void) => (event: Event) => {
      if (this.source === source && !this.stopped) handle(event);
    };
    source.addEventListener(
      'open',
      ifCurrent(() => {
        this.setConnection({ state: 'catching_up' });
      }),
    );
    source.addEventListener(
      'error',
      ifCurrent(() => {
        this.streamError(source);
      }),
    );
    for (const name of STREAM_EVENT_NAMES) {
      source.addEventListener(
        name,
        ifCurrent((event) => {
          this.received(name, event);
        }),
      );
    }
  }

  private received(name: string, event: Event): void {
    let message: StreamMessage;
    try {
      message = decodeMessage(name, (event as MessageEvent<string>).data);
    } catch {
      this.reload('malformed');
      return;
    }
    switch (message.type) {
      case 'reset':
        this.reload(message.reset.reason);
        return;
      case 'live':
        this.failures = 0;
        this.setConnection({ state: 'live' });
        return;
      default:
        this.applyChange(message);
    }
  }

  private applyChange(change: Exclude<StreamMessage, { type: 'reset' | 'live' }>): void {
    if (!this.store?.apply(change)) {
      this.reload('out_of_order');
      return;
    }
    if (change.type === 'end') {
      this.closeStream();
      this.connection = { state: 'ended', reason: change.end.reason };
    }
    this.notify();
  }

  /** The stream broke: the browser reconnects on its own unless the server refused it. */
  private streamError(source: StreamSource): void {
    if (source.readyState === CONNECTING) {
      this.setConnection({ state: 'reconnecting' });
      return;
    }
    if (source.readyState !== CLOSED) return;
    this.closeStream();
    // The snapshot says why: gone, unavailable, sign-in. Its epoch keeps the state if unchanged.
    const retryInMs = Math.min(RETRY_MAX_MS, RETRY_BASE_MS * 2 ** this.failures);
    this.failures++;
    this.setConnection({ state: 'reconnecting' });
    this.retryIn(retryInMs);
  }

  /** Discards the state: the snapshot is loaded again, the stream reopened from it. */
  private reload(reason: ReloadReason): void {
    this.closeStream();
    this.discard = true;
    this.setConnection({ state: 'reloading', reason });
    void this.load();
  }

  private setConnection(connection: Connection): void {
    this.connection = connection;
    this.notify();
  }

  private closeStream(): void {
    this.source?.close();
    this.source = undefined;
  }

  private notify(): void {
    if (this.scheduled || this.stopped) return;
    this.scheduled = true;
    const schedule = this.options.schedule ?? ((flush) => requestAnimationFrame(flush));
    schedule(() => {
      this.scheduled = false;
      if (!this.stopped) this.options.onchange(this.page());
    });
  }

  private page(): LivePage {
    if (this.store) return { phase: 'ready', view: this.store.view(), connection: this.connection };
    if (this.failure) return { phase: 'failed', ...this.failure };
    return { phase: 'loading' };
  }
}

function failureOf(load: Extract<SnapshotLoad, { ok: false }>): LoadFailure {
  return {
    status: load.status,
    message: load.message,
    ...(load.code === undefined ? {} : { code: load.code }),
    ...(load.signIn ? { signIn: true } : {}),
  };
}

/** A network failure or a server error may pass; a missing run, a refusal or a sign-in will not. */
function retryable(failure: LoadFailure): boolean {
  if (failure.signIn) return false;
  if (failure.status === undefined) return true;
  return failure.status >= SERVER_ERROR && failure.status !== NOT_IMPLEMENTED;
}
