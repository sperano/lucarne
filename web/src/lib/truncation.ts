// The limit an event's truncated field was cut to, by the payload field
// names the adapters mark (contract/README.md, "Limits").
import type { Limits } from './contract';

type LimitName = keyof Limits;

const FIELD_LIMITS: Readonly<Record<string, LimitName>> = {
  text: 'maxTextBytes',
  message: 'maxTextBytes',
  detail: 'maxTextBytes',
  outputs: 'maxTextBytes',
  output: 'maxToolOutputBytes',
  input: 'maxToolInputBytes',
  inputText: 'maxToolInputBytes',
  diff: 'maxDiffBytes',
  before: 'maxDiffBytes',
  after: 'maxDiffBytes',
  path: 'maxPathBytes',
};

/** The byte limit field was cut to: a label's when the field is not a payload text. */
export function fieldLimit(field: string, limits: Limits): number {
  return limits[FIELD_LIMITS[field] ?? 'maxLabelBytes'];
}
