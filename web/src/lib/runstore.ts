// A run as the client holds it: the snapshot it loaded, with the stream's
// updates applied in seq order, and its own bound on what it retains. The
// server bounds the snapshot; the client bounds what the stream adds to it
// the same way, dropping the oldest events first and counting them.
import type {
  Attempt,
  AttemptUpdate,
  End,
  Limits,
  Run,
  RunUpdate,
  Snapshot,
  Stream,
  TranscriptEvent,
} from './contract';
import { ConversationBuilder, type Conversation } from './conversation';

/** An update of a run's stream, decoded; size is the length of its data. */
export type RunChange =
  | { readonly type: 'transcript'; readonly event: TranscriptEvent; readonly size: number }
  | { readonly type: 'attempt'; readonly update: AttemptUpdate }
  | { readonly type: 'run'; readonly update: RunUpdate }
  | { readonly type: 'end'; readonly end: End };

/**
 * Once over a bound, events are dropped until this fraction of it is free,
 * so the conversations are rebuilt once per batch of drops, not per event.
 */
export const DROP_FRACTION = 0.05;

/** What the conversation renders of a run at one moment; replaced, never changed. */
export interface RunView {
  readonly run: Run;
  readonly attempts: readonly Attempt[];
  /** The stream's position; droppedEvents counts the server's and the client's drops. */
  readonly stream: Stream;
  readonly limits: Limits;
  readonly end?: End;
  /** The conversation of an attempt, the same object while it does not change. */
  readonly conversationOf: (attemptId: string) => Conversation;
}

export class RunStore {
  private run: Run;
  private attempts: Attempt[];
  private readonly events: TranscriptEvent[] = [];
  private readonly sizes: number[] = [];
  private readonly ids = new Set<string>();
  private retainedSize = 0;
  private readonly epochName: string;
  private lastSeq: number;
  private dropped: number;
  private end: End | undefined;
  private readonly limits: Limits;
  private builders = new Map<string, ConversationBuilder>();
  private current: RunView | undefined;

  constructor(snapshot: Snapshot) {
    this.run = snapshot.run;
    this.attempts = [...snapshot.attempts];
    this.epochName = snapshot.stream.epoch;
    this.lastSeq = snapshot.stream.lastSeq;
    this.dropped = snapshot.stream.droppedEvents;
    this.limits = snapshot.limits;
    for (const event of snapshot.events) this.retain(event, JSON.stringify(event).length);
    this.bound();
  }

  /** The stream cursor after everything applied: `{epoch}:{seq}`. */
  get cursor(): string {
    return `${this.epochName}:${String(this.lastSeq)}`;
  }

  /** The server-side history the store follows. */
  get epoch(): string {
    return this.epochName;
  }

  /**
   * Applies the next update. It returns false, changing nothing, when the
   * update's seq is not the last plus one: something was lost, and the
   * client must reload the snapshot.
   */
  apply(change: RunChange): boolean {
    if (seqOf(change) !== this.lastSeq + 1 || this.end !== undefined) return false;
    this.lastSeq++;
    this.current = undefined;
    switch (change.type) {
      case 'transcript':
        this.retain(change.event, change.size);
        this.bound();
        return true;
      case 'attempt':
        this.updateAttempt(change.update.attempt);
        return true;
      case 'run':
        this.run = change.update.run;
        return true;
      case 'end':
        this.end = change.end;
        return true;
    }
  }

  /** The run as it is now, the same object until the next update. */
  view(): RunView {
    this.current ??= {
      run: this.run,
      attempts: this.attempts.slice(),
      stream: { epoch: this.epochName, lastSeq: this.lastSeq, droppedEvents: this.dropped },
      limits: this.limits,
      ...(this.end === undefined ? {} : { end: this.end }),
      conversationOf: (id) => this.builderOf(id).view(),
    };
    return this.current;
  }

  /** Adds event, unless its ID is already held: an event is shown once. */
  private retain(event: TranscriptEvent, size: number): void {
    if (this.ids.has(event.id)) return;
    this.ids.add(event.id);
    this.events.push(event);
    this.sizes.push(size);
    this.retainedSize += size;
    this.builders.get(event.attempt)?.add(event);
  }

  private updateAttempt(attempt: Attempt): void {
    const index = this.attempts.findIndex((a) => a.id === attempt.id);
    if (index === -1) this.attempts.push(attempt);
    else this.attempts[index] = attempt;
  }

  /** Drops the oldest events while over maxEventsPerRun or maxRunBytes. */
  private bound(): void {
    const { maxEventsPerRun, maxRunBytes } = this.limits;
    if (this.events.length <= maxEventsPerRun && this.retainedSize <= maxRunBytes) return;
    const maxEvents = Math.floor(maxEventsPerRun * (1 - DROP_FRACTION));
    const maxSize = Math.floor(maxRunBytes * (1 - DROP_FRACTION));
    let count = 0;
    let size = this.retainedSize;
    while (
      count < this.events.length &&
      (this.events.length - count > maxEvents || size > maxSize)
    ) {
      size -= this.sizes[count] ?? 0;
      count++;
    }
    for (const event of this.events.splice(0, count)) this.ids.delete(event.id);
    this.sizes.splice(0, count);
    this.retainedSize = size;
    this.dropped += count;
    // A dropped call takes its grouping with it: rebuild what is left.
    this.builders = new Map();
  }

  private builderOf(attemptId: string): ConversationBuilder {
    let builder = this.builders.get(attemptId);
    if (!builder) {
      builder = new ConversationBuilder();
      for (const event of this.events) if (event.attempt === attemptId) builder.add(event);
      this.builders.set(attemptId, builder);
    }
    return builder;
  }
}

function seqOf(change: RunChange): number {
  switch (change.type) {
    case 'transcript':
      return change.event.seq;
    case 'attempt':
    case 'run':
      return change.update.seq;
    case 'end':
      return change.end.seq;
  }
}
