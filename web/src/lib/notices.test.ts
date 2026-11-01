import { describe, expect, it } from 'vitest';
import { eventMaker, type EventBody } from '../dev/events';
import { buildConversation } from './conversation';
import {
  INLINE_DETAIL_CHARS,
  detailLine,
  isDamaged,
  isRunnerNotice,
  noticeName,
  withoutNotices,
} from './notices';

const make = eventMaker();

function notice(isError?: boolean): EventBody {
  return {
    kind: 'lifecycle',
    lifecycle: {
      phase: 'runner_notice',
      subtype: 'hook_response',
      ...(isError ? { isError } : {}),
    },
  };
}

const raw = (reason: 'text' | 'malformed_json' | 'unknown_type' | 'unsupported' | 'oversized') =>
  make({ kind: 'raw', raw: { reason, text: '{}', bytes: 2 } });

describe('isRunnerNotice', () => {
  it('is a runner notice, unless it reports an error', () => {
    expect(isRunnerNotice(make(notice()))).toBe(true);
    expect(isRunnerNotice(make(notice(true)))).toBe(false);
  });

  it('is a well-formed line no adapter rendered, never damage or plain output', () => {
    expect(isRunnerNotice(raw('unknown_type'))).toBe(true);
    expect(isRunnerNotice(raw('unsupported'))).toBe(true);
    expect(isRunnerNotice(raw('malformed_json'))).toBe(false);
    expect(isRunnerNotice(raw('oversized'))).toBe(false);
    expect(isRunnerNotice(raw('text'))).toBe(false);
  });

  it('is not another lifecycle phase', () => {
    expect(isRunnerNotice(make({ kind: 'lifecycle', lifecycle: { phase: 'compacted' } }))).toBe(
      false,
    );
  });
});

describe('isDamaged', () => {
  it('is a cut or oversized line', () => {
    const damaged = (e: ReturnType<typeof raw>) => e.kind === 'raw' && isDamaged(e);
    expect(damaged(raw('malformed_json'))).toBe(true);
    expect(damaged(raw('oversized'))).toBe(true);
    expect(damaged(raw('unknown_type'))).toBe(false);
  });
});

describe('withoutNotices', () => {
  const events = [
    make({ kind: 'message', message: { role: 'assistant', type: 'text', text: 'hi' } }),
    make(notice()),
    make(notice(true)),
    raw('unsupported'),
    raw('malformed_json'),
  ];
  const { items } = buildConversation(events, events[0]?.attempt ?? '');

  it('leaves out the notices, keeping errors and damage', () => {
    expect(withoutNotices(items).map((i) => i.id)).toEqual([
      events[0]?.id,
      events[2]?.id,
      events[4]?.id,
    ]);
  });

  it('keeps the notice a link names', () => {
    const kept = events[1]?.id;
    expect(withoutNotices(items, kept).map((i) => i.id)).toContain(kept);
  });
});

describe('noticeName', () => {
  it('names the line type and the subtype when they differ', () => {
    expect(noticeName('system', 'hook_response')).toBe('system · hook_response');
    expect(noticeName('tool_use_summary', 'tool_use_summary')).toBe('tool_use_summary');
    expect(noticeName(undefined, 'status')).toBe('status');
    expect(noticeName('system', undefined)).toBe('system');
    expect(noticeName(undefined, undefined)).toBe('notice');
  });
});

describe('detailLine', () => {
  it('shows a short single line whole', () => {
    expect(detailLine('compacting')).toEqual({ first: 'compacting', more: false });
    expect(detailLine(undefined)).toEqual({ first: '', more: false });
  });

  it('opens more lines, or a long one, on demand', () => {
    expect(detailLine('PreToolUse:Bash\n\nallowed')).toEqual({
      first: 'PreToolUse:Bash',
      more: true,
    });
    expect(detailLine('a'.repeat(INLINE_DETAIL_CHARS + 1)).more).toBe(true);
  });
});
