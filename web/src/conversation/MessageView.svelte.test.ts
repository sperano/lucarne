import { describe, expect, it } from 'vitest';
import type { Media } from '../lib/contract';
import type { EventOf } from '../lib/conversation';
import { eventMaker } from '../dev/events';
import { LIMITS } from '../dev/fixtures';
import { render } from '../render';
import MessageView from './MessageView.svelte';

const MEDIA_BYTES = 12 * 1024;

function show(text: string, media: readonly Media[] = []): HTMLElement {
  const event = eventMaker()({
    kind: 'message',
    message: { role: 'user', type: 'text', text, ...(media.length > 0 && { media }) },
  }) as EventOf<'message'>;
  return render(MessageView, { event, limits: LIMITS });
}

function entries(root: HTMLElement): (string | null)[] {
  return [...root.querySelectorAll('[aria-label="Images and documents"] li')].map(
    (li) => li.textContent,
  );
}

describe('MessageView', () => {
  it('lists the media after the text, with what the source did not give named as missing', () => {
    const root = show('The button overlaps the footer.', [
      { type: 'image', mediaType: 'image/png', byteSize: MEDIA_BYTES },
      { type: 'document', title: 'Spec' },
    ]);
    expect(root.textContent).toContain('The button overlaps the footer.');
    expect(entries(root)).toEqual([
      'image, image/png, 12 KiB',
      'document "Spec", media type not given, size not given',
    ]);
  });

  it('shows a message of media alone without calling it empty', () => {
    const root = show('', [{ type: 'image' }]);
    expect(entries(root)).toEqual(['image, media type not given, size not given']);
    expect(root.textContent).not.toContain('The message is empty.');
  });

  it('says a message without text or media is empty', () => {
    const root = show('');
    expect(entries(root)).toEqual([]);
    expect(root.textContent).toContain('The message is empty.');
  });
});
