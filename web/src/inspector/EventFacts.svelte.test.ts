import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { ATTEMPT_ID, eventMaker, type EventBody } from '../dev/events';
import { FIXTURE_KEY, snapshotOf } from '../dev/fixtures';
import { render } from '../render';
import EventFacts from './EventFacts.svelte';

function show(body: EventBody) {
  const event = eventMaker()(body);
  const [attempt] = snapshotOf('claude-code', 'example-run', [event]).attempts;
  if (!attempt) throw new Error('no attempt');
  const root = render(EventFacts, { event, attempt, runKey: FIXTURE_KEY });
  const facts = Object.fromEntries(
    [...root.querySelectorAll('dt')].map((dt) => [
      dt.textContent.trim(),
      dt.nextElementSibling?.textContent.replace(/\s+/g, ' ').trim(),
    ]),
  );
  return { root, facts, event };
}

describe('EventFacts', () => {
  it('says an exit code the runner did not report is not reported', () => {
    const { facts } = show({ kind: 'tool_result', toolResult: { callId: 'c', status: 'ok' } });
    expect(facts.Status).toBe('ok');
    expect(facts['Exit code']).toBe('Not reported');
    expect(facts['Source type']).toBe('None (not a JSON event of the runner)');
    expect(facts['Log time']).toBe('Not in the log');
  });

  it('says what an edit gave of its change', () => {
    const given = show({
      kind: 'edit',
      edit: { path: 'a.go', change: 'update', diff: '@@ -1 +1 @@', after: 'x' },
    });
    expect(given.facts['Change given']).toBe('a diff, the text after');
    const bare = show({ kind: 'edit', edit: { path: 'a.go', change: 'unknown' } });
    expect(bare.facts['Change given']).toBe('No diff and no text: only the path changed is known');
  });

  it('says where the attempt and the log stand, and the line of a gap', () => {
    const { facts } = show({ kind: 'gap', gap: { reason: 'read_interrupted' } });
    expect(facts.Attempt).toContain('container claude-code, restart 0');
    expect(facts.Log).toBe('Log read to the end: 1 line read, 1 gap, 0 lines cut');
    expect(facts.Line).toBe('Before line 1, gap 1 there of the read from e3b0c44298fc1c14');
  });

  it('copies a link that opens the event in the inspector', async () => {
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const { root, event } = show({ kind: 'raw', raw: { reason: 'text', text: 'x', bytes: 1 } });
    const button = [...root.querySelectorAll('button')].find(
      (b) => b.textContent === 'Copy the link',
    );
    if (!button) throw new Error('no link button');
    await userEvent.click(button);
    const link = new URL(write.mock.calls[0]?.[0] ?? '');
    expect(link.searchParams.get('view')).toBe('inspector');
    expect(link.searchParams.get('attempt')).toBe(ATTEMPT_ID);
    expect(link.hash).toBe(`#event=${event.id}`);
    vi.restoreAllMocks();
  });
});
