import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { Coverage } from '../lib/contract';
import { buildConversation } from '../lib/conversation';
import { eventMaker, type EventBody } from '../dev/events';
import { snapshotOf } from '../dev/fixtures';
import { render } from '../render';
import Correlation from './Correlation.svelte';

function show(bodies: readonly EventBody[], index: number, coverage: Partial<Coverage> = {}) {
  const events = bodies.map(eventMaker());
  const snapshot = snapshotOf('claude-code', 'example-run', events);
  const [base] = snapshot.attempts;
  const event = events[index];
  if (!base || !event) throw new Error('no attempt or event');
  const attempt = { ...base, coverage: { ...base.coverage, ...coverage } };
  const onselect = vi.fn();
  const root = render(Correlation, {
    event,
    conversation: buildConversation(events, attempt.id),
    attempt,
    stream: snapshot.stream,
    onselect,
  });
  return { root, events, onselect };
}

const BASH: EventBody = {
  kind: 'tool_call',
  toolCall: { callId: 'c1', name: 'Bash', input: { command: 'go test ./...' } },
};

describe('Correlation', () => {
  it('lists the call, its result and its edit, and selects one', async () => {
    const { root, events, onselect } = show(
      [
        BASH,
        { kind: 'tool_result', toolResult: { callId: 'c1', status: 'error', exitCode: 1 } },
        {
          kind: 'edit',
          edit: { callId: 'c1', path: 'a.go', change: 'update', diff: '@@ -1 +1 @@' },
        },
      ],
      0,
    );
    const items = [...root.querySelectorAll('li')];
    expect(items.map((li) => li.querySelector('.relation')?.textContent)).toEqual([
      'Call',
      'Result',
      'Edit',
    ]);
    expect(items[0]?.querySelector('[aria-current=true]')?.textContent).toContain('this event');
    expect(items[1]?.querySelector('.status')?.textContent).toBe('error, exit 1');
    const toResult = items[1]?.querySelector('button');
    if (!toResult) throw new Error('no button to the result');
    await userEvent.click(toResult);
    expect(onselect).toHaveBeenCalledWith(events[1]?.id);
  });

  it('says why a call has no result rather than leaving it unsaid', () => {
    const { root } = show([BASH], 0, { state: 'ended', gaps: 1 });
    expect(root.querySelector('[role=note]')?.textContent).toContain(
      'No result in the log as read: it has 1 gap',
    );
    const ended = show([BASH], 0, { state: 'expired', reason: 'pod deleted' });
    expect(ended.root.textContent).toContain('the log is gone (pod deleted)');
  });

  it('shows nothing for an event without a call', () => {
    const { root } = show(
      [{ kind: 'message', message: { role: 'user', type: 'text', text: 'hi' } }],
      0,
    );
    expect(root.querySelector('section')).toBeNull();
  });
});
