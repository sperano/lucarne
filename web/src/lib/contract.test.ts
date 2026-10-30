import { readdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import schema from '../../../contract/v1/transcript.schema.json';
import {
  CONTAINER_STATES,
  COVERAGE_STATES,
  EDIT_CHANGES,
  END_REASONS,
  ERROR_CODES,
  EVENT_KINDS,
  GAP_REASONS,
  LIFECYCLE_PHASES,
  MESSAGE_ROLES,
  MESSAGE_TYPES,
  RAW_REASONS,
  RESET_REASONS,
  RUN_PHASES,
  RUNNER_KINDS,
  SCHEMA_VERSION,
  TOOL_PROGRESS_PHASES,
  TOOL_RESULT_STATUSES,
  USAGE_SCOPES,
  type Attempt,
  type AttemptKey,
  type AttemptUpdate,
  type ContainerState,
  type ContractInfo,
  type Coverage,
  type DocumentName,
  type Edit,
  type End,
  type ErrorInfo,
  type EventBase,
  type EventPayloads,
  type Gap,
  type Lifecycle,
  type Limits,
  type Live,
  type Message,
  type Output,
  type Raw,
  type Reset,
  type Retry,
  type Run,
  type RunKey,
  type RunList,
  type Runner,
  type RunUpdate,
  type Snapshot,
  type Source,
  type SourceLine,
  type Stream,
  type ToolCall,
  type ToolProgress,
  type ToolResult,
  type Usage,
} from './contract';
import { parseDocument, schemaErrors } from './validate';

const CONTRACT_DIR = resolve(import.meta.dirname, '../../../contract/v1');

interface Example {
  name: DocumentName;
  file: string;
  document: unknown;
}

const DOCUMENT_NAMES: Record<DocumentName, true> = {
  Snapshot: true,
  Event: true,
  AttemptUpdate: true,
  RunUpdate: true,
  Reset: true,
  End: true,
  Live: true,
  ErrorResponse: true,
  ContractInfo: true,
  SourceLine: true,
  RunList: true,
};

function isDocumentName(name: string): name is DocumentName {
  return Object.hasOwn(DOCUMENT_NAMES, name);
}

function loadExamples(kind: 'examples' | 'invalid'): Example[] {
  const root = join(CONTRACT_DIR, kind);
  return readdirSync(root).flatMap((name) => {
    if (!isDocumentName(name)) throw new Error(`${kind}/${name} is not a document definition`);
    return readdirSync(join(root, name)).map((file) => ({
      name,
      file: `${kind}/${name}/${file}`,
      document: JSON.parse(readFileSync(join(root, name, file), 'utf8')) as unknown,
    }));
  });
}

describe('shared examples', () => {
  it.each(loadExamples('examples'))('$file validates', ({ name, document }) => {
    expect(schemaErrors(name, document)).toEqual([]);
  });

  it.each(loadExamples('invalid'))('$file is rejected', ({ name, document }) => {
    expect(schemaErrors(name, document)).not.toEqual([]);
    expect(() => parseDocument(name, document)).toThrow(`invalid ${name}`);
  });

  it('covers every document definition', () => {
    const names = new Set(loadExamples('examples').map((e) => e.name));
    expect([...names].sort()).toEqual(Object.keys(DOCUMENT_NAMES).sort());
  });
});

type Defs = Record<string, { properties?: Record<string, unknown>; required?: string[] }>;
// The JSON import is typed per definition literal; every $defs entry is a JSON Schema object, of which Defs reads only properties and required.
const defs = schema.$defs as unknown as Defs;

/** Each key of T, true when T requires it: checked against T by the compiler. */
type RequiredKeys<T> = { [K in keyof T]-?: object extends Pick<T, K> ? false : true };

function fieldsOf(def: string): Record<string, boolean> {
  const { properties = {}, required = [] } = defs[def] ?? {};
  return Object.fromEntries(Object.keys(properties).map((k) => [k, required.includes(k)]));
}

const objects: Record<string, Record<string, boolean>> = {
  RunKey: { cluster: true, namespace: true, taskUID: true } satisfies RequiredKeys<RunKey>,
  AttemptKey: { podUID: true, container: true, restart: true } satisfies RequiredKeys<AttemptKey>,
  Run: {
    key: true,
    taskName: true,
    runner: false,
    phase: true,
    deleted: false,
    created: false,
    started: false,
    completed: false,
  } satisfies RequiredKeys<Run>,
  Runner: { kind: true, version: false } satisfies RequiredKeys<Runner>,
  ContainerState: {
    state: true,
    exitCode: false,
    reason: false,
    startedAt: false,
    finishedAt: false,
  } satisfies RequiredKeys<ContainerState>,
  Coverage: {
    state: true,
    lines: true,
    gaps: true,
    truncatedLines: true,
    reason: false,
  } satisfies RequiredKeys<Coverage>,
  Attempt: {
    id: true,
    key: true,
    podName: true,
    runner: true,
    container: true,
    coverage: true,
  } satisfies RequiredKeys<Attempt>,
  Stream: { epoch: true, lastSeq: true, droppedEvents: true } satisfies RequiredKeys<Stream>,
  Limits: {
    maxLineBytes: true,
    maxAttemptLogBytes: true,
    maxTextBytes: true,
    maxToolInputBytes: true,
    maxToolOutputBytes: true,
    maxDiffBytes: true,
    maxRawBytes: true,
    maxPathBytes: true,
    maxLabelBytes: true,
    maxEventsPerRun: true,
    maxRunBytes: true,
    maxAttemptsPerRun: true,
    maxReplayUpdates: true,
    maxClientQueue: true,
    maxSourceBytes: true,
    maxRunSourceBytes: true,
    maxListedRuns: true,
  } satisfies RequiredKeys<Limits>,
  Source: {
    line: true,
    part: true,
    type: false,
    logTime: false,
    runnerTime: false,
  } satisfies RequiredKeys<Source>,
  Message: { role: true, type: true, text: true, messageId: false } satisfies RequiredKeys<Message>,
  ToolCall: {
    callId: true,
    name: true,
    input: false,
    inputText: false,
    title: false,
    parentCallId: false,
  } satisfies RequiredKeys<ToolCall>,
  ToolProgress: {
    callId: true,
    phase: true,
    elapsedMs: false,
    detail: false,
    subtype: false,
    background: false,
  } satisfies RequiredKeys<ToolProgress>,
  ToolResult: {
    callId: true,
    status: true,
    output: false,
    exitCode: false,
  } satisfies RequiredKeys<ToolResult>,
  Edit: {
    callId: false,
    path: true,
    change: true,
    diff: false,
    before: false,
    after: false,
  } satisfies RequiredKeys<Edit>,
  Usage: {
    scope: true,
    inputTokens: false,
    outputTokens: false,
    cacheReadTokens: false,
    cacheWriteTokens: false,
    reasoningTokens: false,
    totalTokens: false,
    costUSD: false,
    durationMs: false,
    apiDurationMs: false,
    turns: false,
  } satisfies RequiredKeys<Usage>,
  Retry: { attempt: true, max: false, delayMs: false } satisfies RequiredKeys<Retry>,
  ErrorInfo: {
    message: true,
    code: false,
    status: false,
    retry: false,
  } satisfies RequiredKeys<ErrorInfo>,
  Output: { key: true, value: true } satisfies RequiredKeys<Output>,
  Lifecycle: {
    phase: true,
    sessionId: false,
    model: false,
    runnerVersion: false,
    subtype: false,
    isError: false,
    stopReason: false,
    exitCode: false,
    detail: false,
    outputs: false,
  } satisfies RequiredKeys<Lifecycle>,
  Raw: { reason: true, text: true, bytes: true } satisfies RequiredKeys<Raw>,
  Gap: { reason: true, lines: false, detail: false } satisfies RequiredKeys<Gap>,
  Event: {
    seq: true,
    id: true,
    attempt: true,
    kind: true,
    source: true,
    truncated: false,
  } satisfies RequiredKeys<EventBase>,
  Snapshot: {
    schema: true,
    run: true,
    attempts: true,
    events: true,
    stream: true,
    limits: true,
  } satisfies RequiredKeys<Snapshot>,
  AttemptUpdate: { seq: true, attempt: true } satisfies RequiredKeys<AttemptUpdate>,
  RunUpdate: { seq: true, run: true } satisfies RequiredKeys<RunUpdate>,
  Reset: { epoch: true, reason: true } satisfies RequiredKeys<Reset>,
  End: { seq: true, reason: true } satisfies RequiredKeys<End>,
  Live: { lastSeq: true } satisfies RequiredKeys<Live>,
  ContractInfo: { schema: true, limits: true } satisfies RequiredKeys<ContractInfo>,
  SourceLine: {
    schema: true,
    attempt: true,
    origin: true,
    line: true,
    text: true,
    bytes: true,
  } satisfies RequiredKeys<SourceLine>,
  RunList: { schema: true, runs: true, omittedRuns: true } satisfies RequiredKeys<RunList>,
};

describe('TypeScript types match the schema', () => {
  it.each(Object.entries(objects))('%s', (def, fields) => {
    expect(fieldsOf(def)).toEqual(fields);
  });

  it('event payloads', () => {
    const payloads: Record<keyof EventPayloads, string> = {
      message: 'message',
      tool_call: 'toolCall',
      tool_progress: 'toolProgress',
      tool_result: 'toolResult',
      edit: 'edit',
      usage: 'usage',
      error: 'error',
      lifecycle: 'lifecycle',
      raw: 'raw',
      gap: 'gap',
    } satisfies { [K in keyof EventPayloads]: keyof EventPayloads[K] };
    // Each Event.oneOf branch is a JSON Schema object whose properties are kind and its payload field; only their names are read.
    const branches = (schema.$defs.Event.oneOf as { properties: Record<string, unknown> }[]).map(
      (b) => Object.keys(b.properties).sort(),
    );
    const expected = Object.entries(payloads).map(([, field]) => [field, 'kind'].sort());
    expect(branches).toEqual(expected);
  });

  const $defs = schema.$defs;
  it.each([
    ['event kinds', $defs.Event.properties.kind.enum, EVENT_KINDS],
    ['run phases', $defs.Run.properties.phase.enum, RUN_PHASES],
    ['runner kinds', $defs.RunnerKind.enum, RUNNER_KINDS],
    ['container states', $defs.ContainerState.properties.state.enum, CONTAINER_STATES],
    ['coverage states', $defs.Coverage.properties.state.enum, COVERAGE_STATES],
    ['message roles', $defs.Message.properties.role.enum, MESSAGE_ROLES],
    ['message types', $defs.Message.properties.type.enum, MESSAGE_TYPES],
    ['tool progress phases', $defs.ToolProgress.properties.phase.enum, TOOL_PROGRESS_PHASES],
    ['tool result statuses', $defs.ToolResult.properties.status.enum, TOOL_RESULT_STATUSES],
    ['edit changes', $defs.Edit.properties.change.enum, EDIT_CHANGES],
    ['usage scopes', $defs.Usage.properties.scope.enum, USAGE_SCOPES],
    ['lifecycle phases', $defs.Lifecycle.properties.phase.enum, LIFECYCLE_PHASES],
    ['raw reasons', $defs.Raw.properties.reason.enum, RAW_REASONS],
    ['gap reasons', $defs.Gap.properties.reason.enum, GAP_REASONS],
    ['reset reasons', $defs.Reset.properties.reason.enum, RESET_REASONS],
    ['end reasons', $defs.End.properties.reason.enum, END_REASONS],
    ['error codes', $defs.ErrorResponse.properties.error.properties.code.enum, ERROR_CODES],
  ] as const)('enum: %s', (_name, schemaEnum, values) => {
    expect(schemaEnum).toEqual([...values]);
  });

  it('schema version', () => {
    expect($defs.SchemaVersion.const).toBe(SCHEMA_VERSION);
  });
});
