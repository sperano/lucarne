// The words the conversation uses for the contract's enumerations. Each map
// is total over its enumeration, so a value added to the contract fails to
// compile here until it has a label.
import type {
  ContainerStateKind,
  EndReason,
  CoverageState,
  EditChange,
  EventKind,
  GapReason,
  LifecyclePhase,
  MediaKind,
  RawReason,
  RunPhase,
  RunnerKind,
  ToolProgressPhase,
  ToolResultStatus,
  UsageScope,
} from './contract';
import type { ReloadReason } from './live';

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

export const EVENT_KIND_LABELS: Record<EventKind, string> = {
  message: 'Message',
  tool_call: 'Tool call',
  tool_progress: 'Tool progress',
  tool_result: 'Tool result',
  edit: 'Edit',
  usage: 'Usage',
  error: 'Error',
  lifecycle: 'Lifecycle',
  raw: 'Raw line',
  gap: 'Gap',
};

export const STATUS_LABELS: Record<ToolResultStatus, string> = {
  ok: 'ok',
  error: 'error',
  declined: 'declined',
  unknown: 'outcome unknown',
};

export const MEDIA_KIND_LABELS: Record<MediaKind, { one: string; many: string }> = {
  image: { one: 'image', many: 'images' },
  document: { one: 'document', many: 'documents' },
};

export const TOOL_PROGRESS_LABELS: Record<ToolProgressPhase, string> = {
  started: 'started',
  running: 'running',
  completed: 'completed',
  failed: 'failed',
  killed: 'killed',
  unknown: 'progress unknown',
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
  thinking: 'Thinking',
  session_completed: 'Session completed',
  rate_limit_status: 'Rate limit status',
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

/** Why the page reloads the run, completing "…, so the run is loaded again." */
export const RELOAD_REASON_LABELS: Record<ReloadReason, string> = {
  epoch_changed: "The server's history of this run restarted",
  replay_expired: 'The server can no longer replay what this page missed',
  client_too_slow: 'This page fell too far behind the stream',
  bad_cursor: 'The server refused the stream position',
  out_of_order: 'An update of the stream was missed',
  malformed: 'An update of the stream could not be read',
};

export const END_REASON_LABELS: Record<EndReason, string> = {
  run_finished: 'The run finished: nothing more will arrive.',
  run_deleted: 'The Task was deleted: nothing more will arrive.',
};
