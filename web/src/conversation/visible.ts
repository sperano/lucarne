// An attachment that calls back while its element is near the viewport, to
// grow the transcript window as the reader scrolls towards its end.
import type { Attachment } from 'svelte/attachments';

/** How far below the viewport the next items start rendering. */
export const PRELOAD_MARGIN = '800px';

/**
 * Calls onVisible whenever node comes within PRELOAD_MARGIN of the viewport.
 * Create it with the window's end as a dependency, so a window that grew
 * while the node stayed visible observes it again and keeps growing.
 */
export function whenVisible(onVisible: () => void): Attachment<HTMLElement> {
  return (node) => {
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onVisible();
      },
      { rootMargin: `0px 0px ${PRELOAD_MARGIN} 0px` },
    );
    observer.observe(node);
    return () => {
      observer.disconnect();
    };
  };
}
