import { beforeEach, describe, expect, it } from 'vitest';
import type { Snapshot, TranscriptEvent } from './contract';
import { ATTEMPT_ID, eventMaker } from '../dev/events';
import { LIMITS, snapshotOf } from '../dev/fixtures';
import { DROP_FRACTION, RunStore, type RunChange } from './runstore';

let make = eventMaker();
const message = (text: string) =>
  make({ kind: 'message', message: { role: 'assistant', type: 'text', text } });

function snapshotWith(events: readonly TranscriptEvent[], limits = LIMITS): Snapshot {
  return { ...snapshotOf('claude-code', 'example-run', events), limits };
}

function change(event: TranscriptEvent, seq = event.seq): RunChange {
  return { type: 'transcript', event: { ...event, seq }, size: JSON.stringify(event).length };
}

function texts(store: RunStore): string[] {
  return store
    .view()
    .conversationOf(ATTEMPT_ID)
    .items.map((i) =>
      i.kind === 'event' && i.event.kind === 'message' ? i.event.message.text : i.kind,
    );
}

describe('RunStore', () => {
  beforeEach(() => {
    make = eventMaker();
  });

  it('applies the stream after the snapshot, in seq order', () => {
    const first = message('one');
    const store = new RunStore(snapshotWith([first]));
    expect(store.cursor).toBe('fixture:1');
    expect(store.apply(change(message('two')))).toBe(true);
    expect(texts(store)).toEqual(['one', 'two']);
    expect(store.cursor).toBe('fixture:2');
  });

  it('refuses an update out of order, changing nothing', () => {
    const store = new RunStore(snapshotWith([message('one')]));
    const before = store.view();
    expect(store.apply(change(message('skipped'), 3))).toBe(false);
    expect(store.apply(change(message('again'), 1))).toBe(false);
    expect(store.view()).toBe(before);
    expect(store.cursor).toBe('fixture:1');
  });

  it('shows an event once even if it comes twice', () => {
    const one = message('one');
    const store = new RunStore(snapshotWith([one]));
    expect(store.apply(change(one, 2))).toBe(true);
    expect(texts(store)).toEqual(['one']);
    expect(store.cursor).toBe('fixture:2');
  });

  it('updates attempts and the run, and applies nothing after the end', () => {
    const snapshot = snapshotWith([message('one')]);
    const store = new RunStore(snapshot);
    const [attempt] = snapshot.attempts;
    if (!attempt) throw new Error('no attempt');
    const retried = { ...attempt, id: attempt.id.replace(/\.0$/, '.1') };
    store.apply({
      type: 'attempt',
      update: { seq: 2, attempt: { ...attempt, podName: 'renamed' } },
    });
    store.apply({ type: 'attempt', update: { seq: 3, attempt: retried } });
    store.apply({ type: 'run', update: { seq: 4, run: { ...snapshot.run, phase: 'failed' } } });
    expect(store.apply({ type: 'end', end: { seq: 5, reason: 'run_finished' } })).toBe(true);
    const view = store.view();
    expect(view.attempts.map((a) => a.podName)).toEqual(['renamed', attempt.podName]);
    expect(view.run.phase).toBe('failed');
    expect(view.end?.reason).toBe('run_finished');
    expect(store.apply(change(message('late'), 6))).toBe(false);
  });

  it('keeps the view and each conversation until they change', () => {
    const store = new RunStore(snapshotWith([message('one')]));
    const view = store.view();
    const conversation = view.conversationOf(ATTEMPT_ID);
    expect(store.view()).toBe(view);
    store.apply({ type: 'run', update: { seq: 2, run: view.run } });
    expect(store.view()).not.toBe(view);
    expect(store.view().conversationOf(ATTEMPT_ID)).toBe(conversation);
  });

  it('drops the oldest events past maxEventsPerRun and counts them', () => {
    const maxEventsPerRun = 20;
    const events = Array.from({ length: maxEventsPerRun }, (_, i) => message(`m${String(i)}`));
    const store = new RunStore(snapshotWith(events, { ...LIMITS, maxEventsPerRun }));
    expect(store.view().stream.droppedEvents).toBe(0);
    store.apply(change(message('over'), events.length + 1));
    const kept = Math.floor(maxEventsPerRun * (1 - DROP_FRACTION));
    const shown = texts(store);
    expect(shown).toHaveLength(kept);
    expect(shown.at(-1)).toBe('over');
    expect(store.view().conversationOf(ATTEMPT_ID).events).toHaveLength(kept);
    expect(store.view().stream.droppedEvents).toBe(maxEventsPerRun + 1 - kept);
  });

  it('drops the oldest events past maxRunBytes', () => {
    const events = [message('a'.repeat(100)), message('b'.repeat(100))];
    const size = JSON.stringify(events[0]).length;
    const store = new RunStore(snapshotWith(events, { ...LIMITS, maxRunBytes: size * 2 }));
    store.apply(change(message('c'.repeat(100))));
    expect(texts(store).map((t) => t[0])).toEqual(['c']);
    expect(store.view().stream.droppedEvents).toBe(2);
  });
});
