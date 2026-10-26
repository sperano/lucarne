// Which attempt the conversation shows: the one the link names, else the one
// its #event= fragment belongs to, else the latest. A link naming an attempt
// the run does not have is said to be wrong, not redirected to another.
import type { Attempt } from './contract';

export type Selection =
  | { readonly attempt: Attempt; readonly missing?: undefined }
  | { readonly attempt?: undefined; readonly missing: string }
  | { readonly attempt?: undefined; readonly missing?: undefined };

/** The attempt ID an event ID starts with. */
export function attemptOfEvent(eventId: string): string {
  return eventId.slice(0, eventId.indexOf(':'));
}

/** Selects the attempt to show; attempts are in the snapshot's order, oldest first. */
export function selectAttempt(
  attempts: readonly Attempt[],
  linked: string | undefined,
  event: string | undefined,
): Selection {
  if (linked !== undefined) {
    const attempt = attempts.find((a) => a.id === linked);
    return attempt ? { attempt } : { missing: linked };
  }
  const ofEvent =
    event === undefined ? undefined : attempts.find((a) => a.id === attemptOfEvent(event));
  const attempt = ofEvent ?? attempts.at(-1);
  return attempt ? { attempt } : {};
}
