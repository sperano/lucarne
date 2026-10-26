// Links in log content: only absolute URLs with a scheme that navigates to
// a document become links, so text an agent printed can never run script
// (javascript:), embed content (data:) or reach local files (file:).

/** Schemes a link from log content may use. */
const SAFE_SCHEMES: ReadonlySet<string> = new Set(['http:', 'https:', 'mailto:']);

/**
 * The normalized href of an absolute URL with a safe scheme, or null.
 * Relative URLs are refused too: in a transcript they name a path of the
 * agent's machine, not of Lucarne.
 */
export function safeHref(raw: string): string | null {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  return SAFE_SCHEMES.has(url.protocol) ? url.href : null;
}
