// The source lines the inspector shows next to an event: loaded from the
// server once per line, the last few kept so that moving through the rows
// does not load them again. Nothing is kept beyond the page.
import type { SourceLoad } from './api';

/** Source lines kept, the least recently shown dropped first. */
export const SOURCE_CACHE_LINES = 64;

const SEPARATOR = ':';
const GAP_PART = 'gap.';

/** Loads the source line of an event of the run shown. */
export type LoadSource = (eventId: string, signal?: AbortSignal) => Promise<SourceLoad>;

/** The line an event ID names: the ID without its part, shared by every event the line produced. */
export function lineOf(eventId: string): string {
  return eventId.slice(0, eventId.lastIndexOf(SEPARATOR));
}

/** Whether eventId is a gap's: it names the line the gap was found before, not one it came from. */
export function isGapId(eventId: string): boolean {
  return eventId.slice(eventId.lastIndexOf(SEPARATOR) + 1).startsWith(GAP_PART);
}

/** The source lines loaded, by line; only lines are kept, never a failure, which may pass. */
export class SourceCache {
  private readonly lines = new Map<string, SourceLoad>();

  constructor(
    private readonly load: LoadSource,
    private readonly size: number = SOURCE_CACHE_LINES,
  ) {}

  /** The kept line of eventId, if any, now the most recently shown. */
  kept(eventId: string): SourceLoad | undefined {
    const key = lineOf(eventId);
    const line = this.lines.get(key);
    if (line !== undefined) {
      this.lines.delete(key);
      this.lines.set(key, line);
    }
    return line;
  }

  /** The line of eventId, kept or loaded; a network failure rejects, as fetch does. */
  async get(eventId: string, signal?: AbortSignal): Promise<SourceLoad> {
    const kept = this.kept(eventId);
    if (kept !== undefined) return kept;
    const loaded = await this.load(eventId, signal);
    if (loaded.ok) {
      this.lines.set(lineOf(eventId), loaded);
      for (const oldest of this.lines.keys()) {
        if (this.lines.size <= this.size) break;
        this.lines.delete(oldest);
      }
    }
    return loaded;
  }
}
