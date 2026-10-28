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

export const CONTAINER_STATES = ['waiting', 'running', 'terminated', 'unknown'] as const;
// The type of ContainerState.state: ContainerStateState would repeat the word, hence Kind; Coverage.state is CoverageState.
export type ContainerStateKind = (typeof CONTAINER_STATES)[number];

export const COVERAGE_STATES = ['pending', 'following', 'ended', 'unavailable', 'expired'] as const;
export type CoverageState = (typeof COVERAGE_STATES)[number];

export const ERROR_CODES = [
  'invalid_run_key',
  'run_not_found',
  'not_implemented',
  'internal',
  'not_found',
  'method_not_allowed',
  'unavailable',
  'invalid_event_id',
  'source_not_found',
] as const;
export type ErrorCode = (typeof ERROR_CODES)[number];

export const MESSAGE_ROLES = ['user', 'assistant'] as const;
export type MessageRole = (typeof MESSAGE_ROLES)[number];

export const MESSAGE_TYPES = ['text', 'reasoning'] as const;
export type MessageType = (typeof MESSAGE_TYPES)[number];

export const TOOL_RESULT_STATUSES = ['ok', 'error', 'declined', 'unknown'] as const;
export type ToolResultStatus = (typeof TOOL_RESULT_STATUSES)[number];

export const EDIT_CHANGES = ['add', 'update', 'delete', 'unknown'] as const;
export type EditChange = (typeof EDIT_CHANGES)[number];

export const USAGE_SCOPES = ['message', 'step', 'turn', 'session'] as const;
export type UsageScope = (typeof USAGE_SCOPES)[number];

export const LIFECYCLE_PHASES = [
  'setup_started',
  'setup_completed',
  'setup_failed',
  'session_started',
  'step_started',
  'step_completed',
  'turn_started',
  'turn_completed',
  'turn_failed',
  'compacted',
  'session_completed',
  'rate_limit_status',
  'limit_hit',
  'kelos_outputs',
] as const;
export type LifecyclePhase = (typeof LIFECYCLE_PHASES)[number];

export const RAW_REASONS = [
  'text',
  'malformed_json',
  'unknown_type',
  'unsupported',
  'oversized',
] as const;
export type RawReason = (typeof RAW_REASONS)[number];

export const GAP_REASONS = [
  'source_truncated',
  'read_interrupted',
  'input_limit',
  'attempt_expired',
] as const;
export type GapReason = (typeof GAP_REASONS)[number];

export const RESET_REASONS = [
  'epoch_changed',
  'replay_expired',
  'client_too_slow',
  'bad_cursor',
] as const;
export type ResetReason = (typeof RESET_REASONS)[number];

/** run_finished: a terminal run phase and every attempt ended or expired. */
export const END_REASONS = ['run_finished', 'run_deleted'] as const;
export type EndReason = (typeof END_REASONS)[number];

/** SSE event names on a run's stream. */
export const SSE_EVENTS = {
  transcript: 'transcript',
  attempt: 'attempt',
  run: 'run',
  reset: 'reset',
  end: 'end',
  live: 'live',
} as const;

/** RFC 3339 in UTC, as the source gave it. */
export type Time = string;

export interface RunKey {
  readonly cluster: string;
  readonly namespace: string;
  readonly taskUID: string;
}

export interface AttemptKey {
  readonly podUID: string;
  readonly container: string;
  readonly restart: number;
}

export interface Run {
  readonly key: RunKey;
  readonly taskName: string;
  readonly runner?: RunnerKind;
  readonly phase: RunPhase;
  readonly deleted?: boolean;
  readonly created?: Time;
  readonly started?: Time;
  readonly completed?: Time;
}

export interface Runner {
  readonly kind: RunnerKind;
  readonly version?: string;
}

export interface ContainerState {
  readonly state: ContainerStateKind;
  readonly exitCode?: number;
  readonly reason?: string;
  readonly startedAt?: Time;
  readonly finishedAt?: Time;
}

export interface Coverage {
  readonly state: CoverageState;
  readonly lines: number;
  readonly gaps: number;
  readonly truncatedLines: number;
  readonly reason?: string;
}

export interface Attempt {
  readonly id: string;
  readonly key: AttemptKey;
  readonly podName: string;
  readonly runner: Runner;
  readonly container: ContainerState;
  readonly coverage: Coverage;
}

export interface Stream {
  readonly epoch: string;
  readonly lastSeq: number;
  readonly droppedEvents: number;
}

export interface Limits {
  readonly maxLineBytes: number;
  readonly maxAttemptLogBytes: number;
  readonly maxTextBytes: number;
  readonly maxToolInputBytes: number;
  readonly maxToolOutputBytes: number;
  readonly maxDiffBytes: number;
  readonly maxRawBytes: number;
  readonly maxPathBytes: number;
  readonly maxLabelBytes: number;
  readonly maxEventsPerRun: number;
  readonly maxRunBytes: number;
  readonly maxAttemptsPerRun: number;
  readonly maxReplayUpdates: number;
  readonly maxClientQueue: number;
  readonly maxSourceBytes: number;
  readonly maxRunSourceBytes: number;
}

export interface Source {
  readonly line: number;
  readonly part: number;
  readonly type?: string;
  readonly logTime?: Time;
  readonly runnerTime?: Time;
}

export interface Message {
  readonly role: MessageRole;
  readonly type: MessageType;
  readonly text: string;
  readonly messageId?: string;
}

export interface ToolCall {
  readonly callId: string;
  readonly name: string;
  readonly input?: unknown;
  readonly inputText?: string;
  readonly title?: string;
  readonly parentCallId?: string;
}

export interface ToolResult {
  readonly callId: string;
  readonly status: ToolResultStatus;
  readonly output?: string;
  readonly exitCode?: number;
}

export interface Edit {
  readonly callId?: string;
  readonly path: string;
  readonly change: EditChange;
  readonly diff?: string;
  readonly before?: string;
  readonly after?: string;
}

export interface Usage {
  readonly scope: UsageScope;
  readonly inputTokens?: number;
  readonly outputTokens?: number;
  readonly cacheReadTokens?: number;
  readonly cacheWriteTokens?: number;
  readonly reasoningTokens?: number;
  readonly totalTokens?: number;
  /** The source's cost as a decimal string, never a float. */
  readonly costUSD?: string;
  readonly durationMs?: number;
  readonly apiDurationMs?: number;
  readonly turns?: number;
}

export interface Retry {
  readonly attempt: number;
  readonly max?: number;
  readonly delayMs?: number;
}

export interface ErrorInfo {
  readonly message: string;
  readonly code?: string;
  readonly status?: number;
  readonly retry?: Retry;
}

export interface Output {
  readonly key: string;
  readonly value: string;
}

export interface Lifecycle {
  readonly phase: LifecyclePhase;
  readonly sessionId?: string;
  readonly model?: string;
  readonly runnerVersion?: string;
  readonly subtype?: string;
  readonly isError?: boolean;
  readonly stopReason?: string;
  readonly exitCode?: number;
  readonly detail?: string;
  readonly outputs?: readonly Output[];
}

export interface Raw {
  readonly reason: RawReason;
  readonly text: string;
  readonly bytes: number;
}

export interface Gap {
  readonly reason: GapReason;
  readonly lines?: number;
  readonly detail?: string;
}

export interface EventBase {
  readonly seq: number;
  readonly id: string;
  readonly attempt: string;
  readonly kind: EventKind;
  readonly source: Source;
  readonly truncated?: readonly string[];
}

/** The payload field of each event kind. */
export interface EventPayloads {
  message: { readonly message: Message };
  tool_call: { readonly toolCall: ToolCall };
  tool_result: { readonly toolResult: ToolResult };
  edit: { readonly edit: Edit };
  usage: { readonly usage: Usage };
  error: { readonly error: ErrorInfo };
  lifecycle: { readonly lifecycle: Lifecycle };
  raw: { readonly raw: Raw };
  gap: { readonly gap: Gap };
}

export type TranscriptEvent = {
  [K in EventKind]: EventBase & { readonly kind: K } & EventPayloads[K];
}[EventKind];

export interface Snapshot {
  readonly schema: typeof SCHEMA_VERSION;
  readonly run: Run;
  readonly attempts: readonly Attempt[];
  readonly events: readonly TranscriptEvent[];
  readonly stream: Stream;
  readonly limits: Limits;
}

export interface AttemptUpdate {
  readonly seq: number;
  readonly attempt: Attempt;
}

export interface RunUpdate {
  readonly seq: number;
  readonly run: Run;
}

export interface Reset {
  readonly epoch: string;
  readonly reason: ResetReason;
}

export interface End {
  readonly seq: number;
  readonly reason: EndReason;
}

/** Sent once after a stream's replay: updates after lastSeq are new. */
export interface Live {
  readonly lastSeq: number;
}

export interface ErrorResponse {
  readonly error: { readonly code: ErrorCode; readonly message: string };
}

/** The log line an event came from, as the runner wrote it. */
export interface SourceLine {
  readonly schema: typeof SCHEMA_VERSION;
  readonly attempt: string;
  readonly origin: string;
  readonly line: number;
  /** The line without its timestamp prefix, cut to maxSourceBytes. */
  readonly text: string;
  /** The whole line's length in bytes: more than text holds when it was cut. */
  readonly bytes: number;
}

export interface ContractInfo {
  readonly schema: typeof SCHEMA_VERSION;
  readonly limits: Limits;
}

/** The document types the schema validates, by definition name. */
export interface Documents {
  Snapshot: Snapshot;
  Event: TranscriptEvent;
  AttemptUpdate: AttemptUpdate;
  RunUpdate: RunUpdate;
  Reset: Reset;
  End: End;
  Live: Live;
  ErrorResponse: ErrorResponse;
  ContractInfo: ContractInfo;
  SourceLine: SourceLine;
}
export type DocumentName = keyof Documents;
