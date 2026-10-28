// The counters of a usage event as the runner reported them, formatted for
// display: the conversation's usage lines and the inspector's run context
// both show them, and neither adds what the runner did not count.
import type { Usage } from './contract';
import { formatCount, formatDuration } from './format';

/** The reported counters of u, in a fixed order; those the runner left out are absent. */
export function usageParts(u: Usage): string[] {
  return [
    count(u.inputTokens, 'input tokens'),
    count(u.outputTokens, 'output tokens'),
    count(u.reasoningTokens, 'reasoning tokens'),
    count(u.cacheReadTokens, 'cache read'),
    count(u.cacheWriteTokens, 'cache write'),
    count(u.totalTokens, 'total tokens'),
    u.costUSD === undefined ? '' : `$${u.costUSD}`,
    u.durationMs === undefined ? '' : formatDuration(u.durationMs),
    u.apiDurationMs === undefined ? '' : `API ${formatDuration(u.apiDurationMs)}`,
    u.turns === undefined ? '' : count(u.turns, u.turns === 1 ? 'turn' : 'turns'),
  ].filter((part) => part !== '');
}

function count(n: number | undefined, unit: string): string {
  return n === undefined ? '' : `${formatCount(n)} ${unit}`;
}
