// What a tool result's media entries say of themselves: only what the
// source gave, the rest named as missing. Their bytes never reach the UI.
import { MEDIA_KINDS, type Media } from './contract';
import { formatBytes, formatCount } from './format';
import { MEDIA_KIND_LABELS } from './labels';

export const NO_MEDIA_TYPE = 'media type not given';
export const NO_BYTE_SIZE = 'size not given';

/** One entry, as "image, image/png, 12 KiB" or 'document "Notes", media type not given, size not given'. */
export function mediaLabel(media: Media): string {
  const kind = MEDIA_KIND_LABELS[media.type].one;
  const name = media.title === undefined ? kind : `${kind} "${media.title}"`;
  const size = media.byteSize === undefined ? NO_BYTE_SIZE : formatBytes(media.byteSize);
  return `${name}, ${media.mediaType ?? NO_MEDIA_TYPE}, ${size}`;
}

/** The entries counted by kind, as "2 images, 1 document"; empty for none. */
export function mediaSummary(media: readonly Media[]): string {
  return MEDIA_KINDS.flatMap((kind) => {
    const n = media.filter((m) => m.type === kind).length;
    if (n === 0) return [];
    const { one, many } = MEDIA_KIND_LABELS[kind];
    return [`${formatCount(n)} ${n === 1 ? one : many}`];
  }).join(', ');
}

/** The texts of the entries a search looks in: their media types and titles. */
export function mediaTexts(media: readonly Media[] | undefined): (string | undefined)[] {
  return (media ?? []).flatMap((m) => [m.mediaType, m.title]);
}
