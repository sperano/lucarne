import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import type { SourceLoad } from '../lib/api';
import { SCHEMA_VERSION } from '../lib/contract';
import { SourceCache, type LoadSource } from '../lib/sources';
import { ATTEMPT_ID } from '../dev/events';
import { LIMITS } from '../dev/fixtures';
import { linesSource } from '../dev/sources';
import { render } from '../render';
import SourceView from './SourceView.svelte';

const ORIGIN = 'e3b0c44298fc1c14';
const idOf = (line: number, part = '0') => `${ATTEMPT_ID}:${ORIGIN}:${String(line)}:${part}`;
const NOT_FOUND = 404;
const FORBIDDEN = 403;

function show(eventId: string, load: LoadSource | undefined, onsignin = () => {}) {
  const sources = load ? new SourceCache(load) : undefined;
  const line = Number(eventId.split(':')[2]);
  return render(SourceView, { eventId, line, sources, limits: LIMITS, onsignin });
}

const said = (root: HTMLElement) => root.textContent.replace(/\s+/g, ' ');
const code = (root: HTMLElement) => root.querySelector('pre')?.textContent ?? '';
const failing =
  (load: SourceLoad): LoadSource =>
  () =>
    Promise.resolve(load);

describe('SourceView', () => {
  it('shows a JSON line formatted, and as written', async () => {
    const line = '{"type":"assistant","message":{"role":"assistant"}}';
    const root = show(idOf(1), linesSource([line]));
    await expect.poll(() => code(root)).toContain('"type": "assistant"');
    expect(said(root)).toContain('JSON');
    const toggle = root.querySelector<HTMLInputElement>('input[type=checkbox]');
    if (!toggle) throw new Error('no toggle');
    await userEvent.click(toggle);
    expect(code(root)).toBe(line);
  });

  it('shows a line that is not JSON as written, never as markup', async () => {
    const line = '<b>npm WARN</b> {"type": cut';
    const root = show(idOf(1), linesSource([line]));
    await expect.poll(() => code(root)).toBe(line);
    expect(root.querySelector('input[type=checkbox], b')).toBeNull();
  });

  it('says a line was cut, and does not parse it', async () => {
    const text = '{"type":"user","content":"start';
    const load = failing({
      ok: true,
      line: {
        schema: SCHEMA_VERSION,
        attempt: ATTEMPT_ID,
        origin: ORIGIN,
        line: 1,
        text,
        bytes: 2_097_152,
      },
    });
    const root = show(idOf(1), load);
    await expect
      .poll(() => said(root))
      .toContain('Only the first 31 B of this 2 MiB line are held');
    expect(code(root)).toBe(text);
  });

  it('asks nothing for a gap, which no line produced', async () => {
    const load = vi.fn(linesSource(['{}']));
    const root = show(idOf(1, 'gap.0'), load);
    expect(said(root)).toContain('A gap comes from no line of the log');
    await new Promise((r) => setTimeout(r, 300));
    expect(load).not.toHaveBeenCalled();
  });

  it('says the server no longer holds a line', async () => {
    const root = show(
      idOf(1),
      failing({ ok: false, status: NOT_FOUND, code: 'source_not_found', message: 'gone' }),
    );
    await expect.poll(() => said(root)).toContain('The server no longer holds this line');
  });

  it('offers to sign in again, and says other failures as they came', async () => {
    const onsignin = vi.fn();
    const root = show(
      idOf(1),
      failing({ ok: false, status: 0, message: 'sign-in required', signIn: true }),
      onsignin,
    );
    const button = await vi.waitFor(() => {
      const b = root.querySelector('button');
      if (!b) throw new Error('no button');
      return b;
    });
    await userEvent.click(button);
    expect(onsignin).toHaveBeenCalledOnce();

    const refused = show(idOf(2), failing({ ok: false, status: FORBIDDEN, message: 'denied' }));
    await expect.poll(() => said(refused)).toContain('could not be loaded (403): denied');
    const offline = show(idOf(3), () => Promise.reject(new TypeError('Failed to fetch')));
    await expect.poll(() => said(offline)).toContain('could not be reached: Failed to fetch');
  });

  it('says where there is no server to ask', () => {
    expect(said(show(idOf(1), undefined))).toContain('No server to ask');
  });
});
