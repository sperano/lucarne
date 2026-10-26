// Incremental rendering of a long transcript: the view renders a window of
// at most WINDOW_MAX items, grown WINDOW_STEP items at a time towards either
// end and trimmed at the other, so a 20 000-event run keeps a bounded DOM.

/** Items added to the window at a time. */
export const WINDOW_STEP = 100;
/** Items the window holds at most. */
export const WINDOW_MAX = 600;

/** An item to bring into view; each new object is a new request, even for the same event. */
export interface Target {
  readonly event: string;
  /** Whether it takes the focus: a link's target does, a search result leaves it in the field. */
  readonly focus: boolean;
}

/** The half-open range [start, end) of items rendered. */
export interface ItemWindow {
  readonly start: number;
  readonly end: number;
}

/** The window at the start of a transcript of total items. */
export function firstWindow(total: number): ItemWindow {
  return { start: 0, end: Math.min(WINDOW_STEP, total) };
}

/** The window at the end of a transcript of total items. */
export function lastWindow(total: number): ItemWindow {
  return { start: Math.max(0, total - WINDOW_STEP), end: total };
}

/** A window of WINDOW_STEP items that holds index, as centred as the ends allow. */
export function windowAround(index: number, total: number): ItemWindow {
  const start = Math.min(Math.max(0, index - WINDOW_STEP / 2), Math.max(0, total - WINDOW_STEP));
  return { start, end: Math.min(total, start + WINDOW_STEP) };
}

/** w grown towards the end, its start trimmed to keep WINDOW_MAX items. */
export function later(w: ItemWindow, total: number): ItemWindow {
  const end = Math.min(total, w.end + WINDOW_STEP);
  return { start: Math.max(w.start, end - WINDOW_MAX), end };
}

/** w grown towards the start, its end trimmed to keep WINDOW_MAX items. */
export function earlier(w: ItemWindow): ItemWindow {
  const start = Math.max(0, w.start - WINDOW_STEP);
  return { start, end: Math.min(w.end, start + WINDOW_MAX) };
}

/**
 * The window that keeps showing the items shown before the list changed
 * (shown, their IDs in order) where they are now, its size unchanged: items
 * added at the end do not enter it, items dropped from the start do not
 * move it. Its start when none of them is left.
 */
export function reanchor(
  shown: readonly string[],
  positionOf: ReadonlyMap<string, number>,
  total: number,
): ItemWindow {
  const size = Math.min(shown.length, total);
  let start = 0;
  for (const [i, id] of shown.entries()) {
    const at = positionOf.get(id);
    if (at !== undefined) {
      start = at - i;
      break;
    }
  }
  start = Math.max(0, Math.min(start, total - size));
  return { start, end: start + size };
}
