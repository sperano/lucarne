// Lucarne transcript and API contract, version 1: the TypeScript mirror of
// contract/v1/transcript.schema.json and the Go package contract.
// contract/README.md is the specification; contract.test.ts keeps the three
// in step.

export const SCHEMA_VERSION = 'lucarne.transcript/v1';
export const API_PREFIX = '/api/v1';

export const EVENT_KINDS = [
  'message',
  'tool_call',
  'tool_result',
  'edit',
  'usage',
  'error',
  'lifecycle',
  'raw',
  'gap',
] as const;
export type EventKind = (typeof EVENT_KINDS)[number];

export const RUN_PHASES = [
  'pending',
  'waiting',
  'running',
  'succeeded',
  'failed',
  'unknown',
] as const;
export type RunPhase = (typeof RUN_PHASES)[number];

export const RUNNER_KINDS = ['claude-code', 'codex', 'opencode', 'unknown'] as const;
export type RunnerKind = (typeof RUNNER_KINDS)[number];

export const COVERAGE_STATES = ['pending', 'following', 'ended', 'unavailable', 'expired'] as const;
export type CoverageState = (typeof COVERAGE_STATES)[number];

export const ERROR_CODES = [
  'invalid_run_key',
  'run_not_found',
  'not_implemented',
  'internal',
  'not_found',
  'method_not_allowed',
] as const;
export type ErrorCode = (typeof ERROR_CODES)[number];

/** SSE event names on a run's stream. */
export const SSE_EVENTS = {
  transcript: 'transcript',
  attempt: 'attempt',
  run: 'run',
  reset: 'reset',
  end: 'end',
} as const;

/** RFC 3339 in UTC, as the source gave it. */
export type Time = string;

export interface RunKey {
  cluster: string;
  namespace: string;
  taskUID: string;
}

export interface AttemptKey {
  podUID: string;
  container: string;
  restart: number;
}

export interface Run {
  key: RunKey;
  taskName: string;
  runner?: RunnerKind;
  phase: RunPhase;
  deleted?: boolean;
  created?: Time;
  started?: Time;
  completed?: Time;
}

export interface Runner {
  kind: RunnerKind;
  version?: string;
}

export interface ContainerState {
  state: 'waiting' | 'running' | 'terminated' | 'unknown';
  exitCode?: number;
  reason?: string;
  startedAt?: Time;
  finishedAt?: Time;
}

export interface Coverage {
  state: CoverageState;
  lines: number;
  gaps: number;
  truncatedLines: number;
  reason?: string;
}

export interface Attempt {
  id: string;
  key: AttemptKey;
  podName: string;
  runner: Runner;
  container: ContainerState;
  coverage: Coverage;
}

export interface Stream {
  epoch: string;
  lastSeq: number;
  droppedEvents: number;
}

export interface Limits {
  maxLineBytes: number;
  maxAttemptLogBytes: number;
  maxTextBytes: number;
  maxToolInputBytes: number;
  maxToolOutputBytes: number;
  maxDiffBytes: number;
  maxRawBytes: number;
  maxPathBytes: number;
  maxLabelBytes: number;
  maxEventsPerRun: number;
  maxRunBytes: number;
  maxAttemptsPerRun: number;
  maxReplayUpdates: number;
  maxClientQueue: number;
}

export interface Source {
  line: number;
  part: number;
  type?: string;
  logTime?: Time;
  runnerTime?: Time;
}

export interface Message {
  role: 'user' | 'assistant';
  type: 'text' | 'reasoning';
  text: string;
  messageId?: string;
}

export interface ToolCall {
  callId: string;
  name: string;
  input?: unknown;
  inputText?: string;
  title?: string;
  parentCallId?: string;
}

export interface ToolResult {
  callId: string;
  status: 'ok' | 'error' | 'declined' | 'unknown';
  output?: string;
  exitCode?: number;
}

export interface Edit {
  callId?: string;
  path: string;
  change: 'add' | 'update' | 'delete' | 'unknown';
  diff?: string;
  before?: string;
  after?: string;
}

export interface Usage {
  scope: 'message' | 'step' | 'turn' | 'session';
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  totalTokens?: number;
  /** The source's cost as a decimal string, never a float. */
  costUSD?: string;
  durationMs?: number;
  apiDurationMs?: number;
  turns?: number;
}

export interface Retry {
  attempt: number;
  max?: number;
  delayMs?: number;
}

export interface ErrorInfo {
  message: string;
  code?: string;
  status?: number;
  retry?: Retry;
}

export interface Output {
  key: string;
  value: string;
}

export interface Lifecycle {
  phase:
    | 'setup_started'
    | 'setup_completed'
    | 'setup_failed'
    | 'session_started'
    | 'turn_started'
    | 'turn_completed'
    | 'turn_failed'
    | 'compacted'
    | 'session_completed'
    | 'limit_hit'
    | 'kelos_outputs';
  sessionId?: string;
  model?: string;
  runnerVersion?: string;
  subtype?: string;
  isError?: boolean;
  stopReason?: string;
  exitCode?: number;
  detail?: string;
  outputs?: Output[];
}

export interface Raw {
  reason: 'text' | 'malformed_json' | 'unknown_type' | 'unsupported' | 'oversized';
  text: string;
  bytes: number;
}

export interface Gap {
  reason: 'source_truncated' | 'read_interrupted' | 'input_limit' | 'attempt_expired';
  lines?: number;
  detail?: string;
}

export interface EventBase {
  seq: number;
  id: string;
  attempt: string;
  kind: EventKind;
  source: Source;
  truncated?: string[];
}

/** The payload field of each event kind. */
export interface EventPayloads {
  message: { message: Message };
  tool_call: { toolCall: ToolCall };
  tool_result: { toolResult: ToolResult };
  edit: { edit: Edit };
  usage: { usage: Usage };
  error: { error: ErrorInfo };
  lifecycle: { lifecycle: Lifecycle };
  raw: { raw: Raw };
  gap: { gap: Gap };
}

export type TranscriptEvent = {
  [K in EventKind]: EventBase & { kind: K } & EventPayloads[K];
}[EventKind];

export interface Snapshot {
  schema: typeof SCHEMA_VERSION;
  run: Run;
  attempts: Attempt[];
  events: TranscriptEvent[];
  stream: Stream;
  limits: Limits;
}

export interface AttemptUpdate {
  seq: number;
  attempt: Attempt;
}

export interface RunUpdate {
  seq: number;
  run: Run;
}

export interface Reset {
  epoch: string;
  reason: 'epoch_changed' | 'replay_expired' | 'client_too_slow' | 'bad_cursor';
}

export interface End {
  seq: number;
  reason: 'attempts_ended' | 'run_deleted';
}

export interface ErrorResponse {
  error: { code: ErrorCode; message: string };
}

export interface ContractInfo {
  schema: typeof SCHEMA_VERSION;
  limits: Limits;
}

/** The document types the schema validates, by definition name. */
export interface Documents {
  Snapshot: Snapshot;
  Event: TranscriptEvent;
  AttemptUpdate: AttemptUpdate;
  RunUpdate: RunUpdate;
  Reset: Reset;
  End: End;
  ErrorResponse: ErrorResponse;
  ContractInfo: ContractInfo;
}
export type DocumentName = keyof Documents;
