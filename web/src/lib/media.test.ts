import { describe, expect, it } from 'vitest';
import { mediaLabel, mediaSummary, mediaTexts } from './media';

const KIB = 1024;

describe('media', () => {
  it('labels an entry with what the source gave', () => {
    expect(mediaLabel({ type: 'image', mediaType: 'image/png', byteSize: 12 * KIB })).toBe(
      'image, image/png, 12 KiB',
    );
    expect(mediaLabel({ type: 'document', mediaType: 'application/pdf', title: 'Notes' })).toBe(
      'document "Notes", application/pdf, size not given',
    );
  });

  it('names what is missing as missing', () => {
    expect(mediaLabel({ type: 'document' })).toBe('document, media type not given, size not given');
    expect(mediaLabel({ type: 'image', byteSize: 0 })).toBe('image, media type not given, 0 B');
  });

  it('counts the entries by kind', () => {
    expect(mediaSummary([])).toBe('');
    expect(mediaSummary([{ type: 'document' }])).toBe('1 document');
    expect(mediaSummary([{ type: 'document' }, { type: 'image' }, { type: 'image' }])).toBe(
      '2 images, 1 document',
    );
  });

  it('gives a search the media types and titles', () => {
    expect(mediaTexts(undefined)).toEqual([]);
    expect(
      mediaTexts([{ type: 'document', mediaType: 'application/pdf', title: 'Notes' }]),
    ).toEqual(['application/pdf', 'Notes']);
  });
});
