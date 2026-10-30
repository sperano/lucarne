import { describe, expect, it } from 'vitest';
import { ATTEMPT_ID, eventMaker, type EventBody } from '../dev/events';
import { buildConversation, type ToolItem } from './conversation';
import {
  effectiveToolOutcome,
  isActiveProgressPhase,
  isFailedProgressPhase,
  isTerminalProgressPhase,
  latestProgressBackground,
  latestProgressElapsed,
} from './tool-progress';

function tool(bodies: readonly EventBody[]): ToolItem {
  const conversation = buildConversation(bodies.map(eventMaker()), ATTEMPT_ID);
  const [item] = conversation.items;
  if (item?.kind !== 'tool') throw new Error('no tool item');
  return item;
}

describe('tool progress state', () => {
  it('classifies progress phases', () => {
    expect(isActiveProgressPhase('started')).toBe(true);
    expect(isActiveProgressPhase('running')).toBe(true);
    expect(isActiveProgressPhase('completed')).toBe(false);
    expect(isTerminalProgressPhase('completed')).toBe(true);
    expect(isTerminalProgressPhase('failed')).toBe(true);
    expect(isTerminalProgressPhase('killed')).toBe(true);
    expect(isTerminalProgressPhase('unknown')).toBe(false);
    expect(isFailedProgressPhase('failed')).toBe(true);
    expect(isFailedProgressPhase('killed')).toBe(true);
    expect(isFailedProgressPhase('completed')).toBe(false);
  });

  it('lets a later terminal notification override an immediate result', () => {
    const item = tool([
      { kind: 'tool_call', toolCall: { callId: 'c', name: 'Task' } },
      { kind: 'tool_result', toolResult: { callId: 'c', status: 'ok' } },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'failed' } },
    ]);
    expect(effectiveToolOutcome(item)?.kind).toBe('tool_progress');
  });

  it('keeps the result when terminal foreground progress came first', () => {
    const item = tool([
      { kind: 'tool_call', toolCall: { callId: 'c', name: 'Bash' } },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'completed' } },
      { kind: 'tool_result', toolResult: { callId: 'c', status: 'error' } },
    ]);
    expect(effectiveToolOutcome(item)?.kind).toBe('tool_result');
  });

  it('keeps the latest reported background and elapsed values', () => {
    const item = tool([
      { kind: 'tool_call', toolCall: { callId: 'c', name: 'Task' } },
      {
        kind: 'tool_progress',
        toolProgress: { callId: 'c', phase: 'started', background: true, elapsedMs: 10 },
      },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'running' } },
      {
        kind: 'tool_progress',
        toolProgress: { callId: 'c', phase: 'running', background: false, elapsedMs: 30 },
      },
      { kind: 'tool_progress', toolProgress: { callId: 'c', phase: 'completed' } },
    ]);
    expect(latestProgressBackground(item.progress)).toBe(false);
    expect(latestProgressElapsed(item.progress)).toBe(30);
  });
});
