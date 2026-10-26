// Display formats of the values a source reported. They format, never
// derive: a duration is shown only when the source gave one.

const MS_PER_SECOND = 1000;
const SECONDS_PER_MINUTE = 60;
const BYTES_PER_KIB = 1024;
const BYTES_PER_MIB = BYTES_PER_KIB * BYTES_PER_KIB;
const SECONDS_DECIMALS = 1;

const integer = new Intl.NumberFormat('en-US');

/** A count with thousands separators. */
export function formatCount(n: number): string {
  return integer.format(n);
}

/** A duration in milliseconds: "850 ms", "48.2 s", "3 min 05 s". */
export function formatDuration(ms: number): string {
  if (ms < MS_PER_SECOND) return `${formatCount(ms)} ms`;
  const seconds = ms / MS_PER_SECOND;
  if (seconds < SECONDS_PER_MINUTE) return `${seconds.toFixed(SECONDS_DECIMALS)} s`;
  const whole = Math.floor(seconds);
  const minutes = Math.floor(whole / SECONDS_PER_MINUTE);
  const rest = String(whole % SECONDS_PER_MINUTE).padStart(2, '0');
  return `${formatCount(minutes)} min ${rest} s`;
}

/** A size in bytes: "512 B", "8 KiB", "1.5 MiB". */
export function formatBytes(n: number): string {
  if (n < BYTES_PER_KIB) return `${formatCount(n)} B`;
  if (n < BYTES_PER_MIB) return `${trim(n / BYTES_PER_KIB)} KiB`;
  return `${trim(n / BYTES_PER_MIB)} MiB`;
}

function trim(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** An RFC 3339 UTC time as written, made readable: "2026-10-25 09:00:02 UTC". */
export function formatTime(time: string): string {
  return time.replace('T', ' ').replace(/Z$/, ' UTC');
}

/** The UTF-8 length of text, which the contract's byte limits count. */
export function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length;
}
