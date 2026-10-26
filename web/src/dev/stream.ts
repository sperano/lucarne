// A scripted stand-in for the browser's EventSource, for tests: each stream
// the page opens is recorded, and the test sends it the server's events,
// breaks it as a network would, or lets the browser reconnect it.
import {
  SSE_EVENTS,
  type Attempt,
  type EndReason,
  type ResetReason,
  type Run,
  type TranscriptEvent,
} from '../lib/contract';
import type { OpenStream, StreamSource } from '../lib/live';

const CONNECTING = 0;
const OPEN = 1;
const CLOSED = 2;

export class FakeStream implements StreamSource {
  readyState = CONNECTING;
  private readonly listeners = new Map<string, ((event: Event) => void)[]>();

  constructor(
    readonly url: string,
    private readonly epoch: string,
  ) {}

  /** The after= cursor the stream was opened with. */
  get after(): string | null {
    return new URL(this.url, 'http://lucarne.invalid').searchParams.get('after');
  }

  get closed(): boolean {
    return this.readyState === CLOSED;
  }

  addEventListener(type: string, listener: (event: Event) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  close(): void {
    this.readyState = CLOSED;
  }

  /** The server answered: the stream is open. */
  open(): this {
    this.readyState = OPEN;
    this.dispatch('open', new Event('open'));
    return this;
  }

  /** Sends an SSE event; transcript, attempt, run and end carry an id. */
  send(name: string, data: unknown, id?: string): this {
    if (this.closed) throw new Error(`${name} sent on a closed stream`);
    const init = { data: JSON.stringify(data), lastEventId: id ?? '' };
    this.dispatch(name, new MessageEvent(name, init));
    return this;
  }

  event(event: TranscriptEvent): this {
    return this.send(SSE_EVENTS.transcript, event, this.id(event.seq));
  }

  attempt(seq: number, attempt: Attempt): this {
    return this.send(SSE_EVENTS.attempt, { seq, attempt }, this.id(seq));
  }

  run(seq: number, run: Run): this {
    return this.send(SSE_EVENTS.run, { seq, run }, this.id(seq));
  }

  end(seq: number, reason: EndReason = 'run_finished'): this {
    return this.send(SSE_EVENTS.end, { seq, reason }, this.id(seq));
  }

  live(lastSeq: number): this {
    return this.send(SSE_EVENTS.live, { lastSeq });
  }

  reset(reason: ResetReason, epoch = this.epoch): this {
    return this.send(SSE_EVENTS.reset, { epoch, reason });
  }

  /** The connection broke: the browser reconnects (true) or gave up (false). */
  fail(reconnecting: boolean): this {
    this.readyState = reconnecting ? CONNECTING : CLOSED;
    this.dispatch('error', new Event('error'));
    return this;
  }

  private id(seq: number): string {
    return `${this.epoch}:${String(seq)}`;
  }

  private dispatch(type: string, event: Event): void {
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
}

/** The streams a page opened, newest last; open is its OpenStream. */
export class FakeStreams {
  readonly opened: FakeStream[] = [];

  constructor(private readonly epoch = 'fixture') {}

  readonly open: OpenStream = (url) => {
    const stream = new FakeStream(url, this.epoch);
    this.opened.push(stream);
    return stream;
  };

  /** The stream opened last, or throws. */
  get last(): FakeStream {
    const stream = this.opened.at(-1);
    if (!stream) throw new Error('no stream opened');
    return stream;
  }
}
