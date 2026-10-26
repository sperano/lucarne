// The words the conversation uses for the contract's enumerations. Each map
// is total over its enumeration, so a value added to the contract fails to
// compile here until it has a label.
import type {
  ContainerStateKind,
  CoverageState,
  EditChange,
  GapReason,
  LifecyclePhase,
  RawReason,
  RunPhase,
  RunnerKind,
  ToolResultStatus,
  UsageScope,
} from './contract';

export const RUN_PHASE_LABELS: Record<RunPhase, string> = {
  pending: 'Pending',
  waiting: 'Waiting',
  running: 'Running',
  succeeded: 'Succeeded',
  failed: 'Failed',
  unknown: 'Unknown',
};

export const RUNNER_LABELS: Record<RunnerKind, string> = {
  'claude-code': 'Claude Code',
  codex: 'Codex',
  opencode: 'OpenCode',
  unknown: 'Unknown runner',
};

export const CONTAINER_STATE_LABELS: Record<ContainerStateKind, string> = {
  waiting: 'waiting',
  running: 'running',
  terminated: 'terminated',
  unknown: 'state unknown',
};

export const COVERAGE_LABELS: Record<CoverageState, string> = {
  pending: 'Log not read yet',
  following: 'Following the log',
  ended: 'Log read to the end',
  unavailable: 'Log unavailable',
  expired: 'Log expired',
};

export const STATUS_LABELS: Record<ToolResultStatus, string> = {
  ok: 'ok',
  error: 'error',
  declined: 'declined',
  unknown: 'outcome unknown',
};

export const EDIT_LABELS: Record<EditChange, string> = {
  add: 'Added',
  update: 'Updated',
  delete: 'Deleted',
  unknown: 'Changed',
};

export const USAGE_SCOPE_LABELS: Record<UsageScope, string> = {
  message: 'message',
  step: 'step',
  turn: 'turn',
  session: 'session',
};

export const LIFECYCLE_LABELS: Record<LifecyclePhase, string> = {
  setup_started: 'Setup started',
  setup_completed: 'Setup completed',
  setup_failed: 'Setup failed',
  session_started: 'Session started',
  step_started: 'Step started',
  step_completed: 'Step completed',
  turn_started: 'Turn started',
  turn_completed: 'Turn completed',
  turn_failed: 'Turn failed',
  compacted: 'Context compacted',
  session_completed: 'Session completed',
  limit_hit: 'Usage limit hit',
  kelos_outputs: 'Kelos outputs',
};

export const RAW_REASON_LABELS: Record<RawReason, string> = {
  text: 'Plain output',
  malformed_json: 'Malformed JSON line',
  unknown_type: 'Event type no adapter knows',
  unsupported: 'Event the adapter does not render',
  oversized: 'Line over the size limit',
};

export const GAP_REASON_LABELS: Record<GapReason, string> = {
  source_truncated: 'The start of the log is gone (rotated).',
  read_interrupted: 'The log stream broke and the reconnect could not prove continuity.',
  input_limit: 'The attempt passed the log size Lucarne reads; the rest is not shown.',
  attempt_expired: 'The pod went away while its log was being read.',
};
