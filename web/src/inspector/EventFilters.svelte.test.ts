import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { NO_FILTER, type EventFilter } from '../lib/filters';
import { render } from '../render';
import EventFilters from './EventFilters.svelte';

function show(filter: EventFilter = NO_FILTER, query = '', current?: number) {
  const onfilter = vi.fn();
  const onstep = vi.fn();
  const onreset = vi.fn();
  const root = render(EventFilters, {
    query,
    filter,
    kinds: [
      ['message', 3],
      ['tool_call', 2],
    ] as const,
    tools: ['Bash', 'Read'],
    listed: 5,
    total: 5,
    matches: query === '' ? 0 : 2,
    current,
    onfilter,
    onstep,
    onreset,
  });
  return { root, onfilter, onstep, onreset };
}

const button = (root: HTMLElement, name: string) => {
  const found = [...root.querySelectorAll('button')].find((b) => b.textContent === name);
  if (!found) throw new Error(`no button ${name}`);
  return found;
};

describe('EventFilters', () => {
  it('offers the kinds present with their counts, and the tools called', () => {
    const { root } = show();
    expect([...root.querySelectorAll('.kind')].map((k) => k.textContent.trim())).toEqual([
      'Message 3',
      'Tool call 2',
    ]);
    expect([...root.querySelectorAll('option')].map((o) => o.textContent)).toEqual([
      'Any tool',
      'Bash',
      'Read',
    ]);
    expect(button(root, 'Reset').disabled).toBe(true);
    expect(root.textContent).toContain('5 events, none filtered out');
  });

  it('changes the filters', async () => {
    const { root, onfilter } = show();
    const kind = root.querySelector<HTMLInputElement>('.kind input');
    if (!kind) throw new Error('no kind');
    await userEvent.click(kind);
    expect(onfilter).toHaveBeenLastCalledWith({ ...NO_FILTER, kinds: ['message'] });
    const select = root.querySelector('select');
    if (!select) throw new Error('no select');
    await userEvent.selectOptions(select, 'Read');
    expect(onfilter).toHaveBeenLastCalledWith({ ...NO_FILTER, tool: 'Read' });
  });

  it('steps through the matches with the buttons and Enter, and resets', async () => {
    const { root, onstep, onreset } = show({ ...NO_FILTER, failures: true }, 'make', 0);
    expect(root.textContent).toContain('1 of 2');
    await userEvent.click(button(root, 'Next'));
    expect(onstep).toHaveBeenLastCalledWith(1);
    const field = root.querySelector('input[type=search]');
    if (!(field instanceof HTMLInputElement)) throw new Error('no search field');
    field.focus();
    await userEvent.keyboard('{Shift>}{Enter}{/Shift}');
    expect(onstep).toHaveBeenLastCalledWith(-1);
    await userEvent.click(button(root, 'Reset'));
    expect(onreset).toHaveBeenCalledOnce();
  });

  it('keeps offering a tool chosen on another attempt', () => {
    const { root } = show({ ...NO_FILTER, tool: 'Grep' });
    expect(root.querySelector('select')?.value).toBe('Grep');
  });
});
