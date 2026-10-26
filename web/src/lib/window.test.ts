import { describe, expect, it } from 'vitest';
import {
  earlier,
  firstWindow,
  lastWindow,
  later,
  WINDOW_MAX,
  WINDOW_STEP,
  windowAround,
} from './window';

const TOTAL = 20000;

describe('item windows', () => {
  it('start and end at the transcript ends', () => {
    expect(firstWindow(TOTAL)).toEqual({ start: 0, end: WINDOW_STEP });
    expect(firstWindow(3)).toEqual({ start: 0, end: 3 });
    expect(lastWindow(TOTAL)).toEqual({ start: TOTAL - WINDOW_STEP, end: TOTAL });
    expect(lastWindow(3)).toEqual({ start: 0, end: 3 });
  });

  it('hold the item they are centred on', () => {
    for (const index of [0, 10, 5000, TOTAL - 1]) {
      const w = windowAround(index, TOTAL);
      expect(w.start).toBeLessThanOrEqual(index);
      expect(w.end).toBeGreaterThan(index);
      expect(w.end - w.start).toBe(WINDOW_STEP);
    }
    expect(windowAround(1, 3)).toEqual({ start: 0, end: 3 });
  });

  it('grow by WINDOW_STEP and never hold more than WINDOW_MAX items', () => {
    let w = firstWindow(TOTAL);
    for (let i = 0; i < TOTAL / WINDOW_STEP + 1; i++) {
      w = later(w, TOTAL);
      expect(w.end - w.start).toBeLessThanOrEqual(WINDOW_MAX);
    }
    expect(w.end).toBe(TOTAL);
    for (let i = 0; i < TOTAL / WINDOW_STEP + 1; i++) {
      w = earlier(w);
      expect(w.end - w.start).toBeLessThanOrEqual(WINDOW_MAX);
    }
    expect(w).toEqual({ start: 0, end: WINDOW_MAX });
  });
});
