import { describe, expect, it } from 'vitest';
import line from '../../../contract/v1/examples/SourceLine/assistant-line.json';
import type { SourceLoad } from './api';
import type { SourceLine } from './contract';
import { isGapId, lineOf, SourceCache } from './sources';

const ATTEMPT = '7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0';
const id = (lineNo: number, part: string) =>
  `${ATTEMPT}:635a06eb36cc2997:${String(lineNo)}:${part}`;
const NOT_FOUND = 404;

function counting(answer: (eventId: string) => SourceLoad) {
  const asked: string[] = [];
  const load = (eventId: string) => {
    asked.push(eventId);
    return Promise.resolve(answer(eventId));
  };
  return { load, asked };
}

const found = (): SourceLoad => ({ ok: true, line: line as SourceLine });

describe('source line identity', () => {
  it('names the line of an event without its part', () => {
    expect(lineOf(id(2, '0'))).toBe(lineOf(id(2, '1')));
    expect(lineOf(id(2, '0'))).not.toBe(lineOf(id(12, '0')));
  });

  it('tells a gap from an event of the line it is before', () => {
    expect(isGapId(id(2, 'gap.0'))).toBe(true);
    expect(isGapId(id(2, '0'))).toBe(false);
  });
});

describe('SourceCache', () => {
  it('loads a line once for every event it produced', async () => {
    const { load, asked } = counting(found);
    const cache = new SourceCache(load);
    expect(await cache.get(id(2, '0'))).toEqual(found());
    expect(await cache.get(id(2, '1'))).toEqual(found());
    expect(asked).toEqual([id(2, '0')]);
    expect(cache.kept(id(2, '3'))).toEqual(found());
  });

  it('keeps no failure, which may pass', async () => {
    const { load, asked } = counting(() => ({
      ok: false,
      status: NOT_FOUND,
      code: 'source_not_found',
      message: 'gone',
    }));
    const cache = new SourceCache(load);
    await cache.get(id(2, '0'));
    await cache.get(id(2, '0'));
    expect(asked).toHaveLength(2);
  });

  it('drops the line shown least recently past its size', async () => {
    const { load, asked } = counting(found);
    const cache = new SourceCache(load, 2);
    await cache.get(id(1, '0'));
    await cache.get(id(2, '0'));
    await cache.get(id(1, '0'));
    await cache.get(id(3, '0'));
    expect(cache.kept(id(1, '0'))).toBeDefined();
    expect(cache.kept(id(2, '0'))).toBeUndefined();
    expect(asked).toEqual([id(1, '0'), id(2, '0'), id(3, '0')]);
  });
});
