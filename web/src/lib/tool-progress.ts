import type { ToolProgressPhase } from './contract';
import type { EventOf, ToolItem } from './conversation';

export function isTerminalProgressPhase(phase: ToolProgressPhase): boolean {
  return phase === 'completed' || isFailedProgressPhase(phase);
}

export function isFailedProgressPhase(phase: ToolProgressPhase): boolean {
  return phase === 'failed' || phase === 'killed';
}

/** The result or progress event that currently describes the call's outcome. */
export function effectiveToolOutcome(
  tool: ToolItem,
): EventOf<'tool_result' | 'tool_progress'> | undefined {
  const progress = tool.progress.at(-1);
  if (progress !== undefined && progressOverridesResult(tool, progress)) return progress;
  return tool.result ?? progress;
}

/** The latest background value the runner reported, preserving an explicit false. */
export function latestProgressBackground(events: ToolItem['progress']): boolean | undefined {
  for (let i = events.length - 1; i >= 0; i--) {
    const background = events[i]?.toolProgress.background;
    if (background !== undefined) return background;
  }
  return undefined;
}

export function latestProgressElapsed(events: ToolItem['progress']): number | undefined {
  for (let i = events.length - 1; i >= 0; i--) {
    const elapsed = events[i]?.toolProgress.elapsedMs;
    if (elapsed !== undefined) return elapsed;
  }
  return undefined;
}

function progressOverridesResult(tool: ToolItem, progress: EventOf<'tool_progress'>): boolean {
  if (!isTerminalProgressPhase(progress.toolProgress.phase)) return false;
  return (
    tool.result === undefined ||
    progress.seq > tool.result.seq ||
    latestProgressBackground(tool.progress) === true
  );
}
