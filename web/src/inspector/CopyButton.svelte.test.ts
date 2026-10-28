import { afterEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from '../render';
import CopyButton from './CopyButton.svelte';

describe('CopyButton', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('copies the text when pressed and says so', async () => {
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    let text = 'first';
    const root = render(CopyButton, { label: 'Copy the ID', text: () => text });
    text = 'second';
    const button = root.querySelector('button');
    if (!button) throw new Error('no button');
    await userEvent.click(button);
    expect(write).toHaveBeenCalledWith('second');
    await expect.poll(() => root.textContent).toContain('Copied');
  });

  it('says when the browser refuses', async () => {
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new DOMException('denied'));
    const root = render(CopyButton, { label: 'Copy', text: () => 'x' });
    root.querySelector('button')?.click();
    await expect.poll(() => root.textContent).toContain('The browser refused to copy');
  });
});
