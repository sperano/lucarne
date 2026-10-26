import { describe, expect, it } from 'vitest';
import attempt from '../../../contract/v1/examples/AttemptUpdate/coverage-ended.json';
import end from '../../../contract/v1/examples/End/run-finished.json';
import event from '../../../contract/v1/examples/Event/message-reasoning.json';
import live from '../../../contract/v1/examples/Live/replay-done.json';
import reset from '../../../contract/v1/examples/Reset/replay-expired.json';
import run from '../../../contract/v1/examples/RunUpdate/succeeded.json';
import { SSE_EVENTS } from './contract';
import { decodeMessage, eventsPath } from './updates';

const encode = (value: unknown) => JSON.stringify(value);

describe('decodeMessage', () => {
  it('decodes each event of a run stream', () => {
    expect(decodeMessage(SSE_EVENTS.transcript, encode(event))).toEqual({
      type: 'transcript',
      event,
      size: encode(event).length,
    });
    expect(decodeMessage(SSE_EVENTS.attempt, encode(attempt))).toEqual({
      type: 'attempt',
      update: attempt,
    });
    expect(decodeMessage(SSE_EVENTS.run, encode(run))).toEqual({ type: 'run', update: run });
    expect(decodeMessage(SSE_EVENTS.end, encode(end))).toEqual({ type: 'end', end });
    expect(decodeMessage(SSE_EVENTS.reset, encode(reset))).toEqual({ type: 'reset', reset });
    expect(decodeMessage(SSE_EVENTS.live, encode(live))).toEqual({ type: 'live', live });
  });

  it.each([
    ['not JSON', SSE_EVENTS.transcript, '{'],
    ['not an object', SSE_EVENTS.run, '7'],
    ['no seq', SSE_EVENTS.transcript, encode({ ...event, seq: undefined })],
    ['a negative seq', SSE_EVENTS.end, encode({ ...end, seq: -1 })],
    ['no attempt', SSE_EVENTS.attempt, encode({ seq: 1 })],
    ['an unknown reset reason', SSE_EVENTS.reset, encode({ ...reset, reason: 'tired' })],
    ['an unknown event name', 'message', encode(event)],
  ])('refuses %s', (_name, name, data) => {
    expect(() => decodeMessage(name, data)).toThrow();
  });
});

describe('eventsPath', () => {
  it('opens the stream of a run after a cursor', () => {
    const key = {
      cluster: 'example',
      namespace: 'agents',
      taskUID: '3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b',
    };
    expect(eventsPath(key, 'a1b2c3d4e5f60718:42')).toBe(
      '/api/v1/runs/example/agents/3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b/events?after=a1b2c3d4e5f60718%3A42',
    );
  });
});
