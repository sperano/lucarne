/**
 * Marks the end of an exhaustive match: a call compiles only once every
 * variant has been handled, so adding a variant fails the typecheck.
 */
export function unreachable(value: never): never {
  throw new Error(`unhandled variant: ${JSON.stringify(value)}`);
}
