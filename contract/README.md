# Transcript and API contract, version 1

`lucarne.transcript/v1` is what the Lucarne server sends and the UI reads.
It is defined three times, kept in step by tests:

| Where | What | Checked by |
| --- | --- | --- |
| [`v1/transcript.schema.json`](v1/transcript.schema.json) | JSON Schema 2020-12; the reference | both test suites |
| this package (`*.go`) | Go types, identities, limits, `Validate` | `go test ./contract/` |
| [`web/src/lib/contract.ts`](../web/src/lib/contract.ts) | TypeScript types and enums | `npm test` in `web/` |

[`v1/examples/<Definition>/`](v1/examples) holds valid documents and
[`v1/invalid/<Definition>/`](v1/invalid) documents that must be rejected,
each named after the one rule it breaks. Both suites validate every example
against the schema. The Go suite also decodes each valid example into its Go
type without unknown fields and re-encodes it unchanged; both suites compare
their types' fields, required fields and enums with the schema. The examples
illustrate the contract; they are not adapter output.

A change to the wire format changes the schema, the Go types, the TypeScript
types and an example in the same pull request. The server and the UI ship in
one binary, so they never disagree; the major version (`v1` in the schema
name and in `/api/v1`) changes only for a change that breaks a link or a
client outside this repository. Within v1 the schema is closed: an unknown
property is an error, not something to ignore.

## Principles

- **Identity is by UID.** A run is a Kelos Task identified by cluster,
  namespace and Task UID, never by its name, which a retry or a later Task
  reuses. A link to an old run never shows a newer one.
- **Only what the source emitted.** Every event comes from a line of an
  attempt's log, or is a gap saying lines are missing. Nothing is inferred:
  no prompt, reasoning, time, cost or outcome the source did not write. A
  field the source did not give is absent, never zero or empty, and one it
  gave empty (a command that printed nothing, an emptied file) stays empty.
- **Loss is explicit.** Truncation, gaps, expired pods and replay that can
  no longer be served are states the client sees, not silent omissions.
- **Bounded.** Every input and output has a named limit (see "Limits").

## Identity

| Name | Form | Example |
| --- | --- | --- |
| cluster | DNS label, configured on the server (Kubernetes has no name for itself) | `example` |
| namespace | DNS label | `agents` |
| Task UID | lowercase UID, `metadata.uid` of the Task | `3f8e2a4c-1b2d-4e5f-8a9b-0c1d2e3f4a5b` |
| attempt ID | `<podUID>.<container>.<restart>` | `7c9d0e1f-2a3b-4c5d-9e6f-7a8b9c0d1e2f.claude-code.0` |
| event ID | `<attemptID>:<line>:<part>`, or `<attemptID>:<line>:gap` | `7c9d…1e2f.claude-code.0:42:1` |

An **attempt** is one container instance: a pod (by UID; a retried Job makes
a second pod, and Kelos's `status.podName` names only the latest), one of
its containers, and that container's restart count when it ran. Kubernetes
keeps the log of the current instance and of the previous one only; older
restarts are `expired`. The pod name is shown, never used as identity.

Neither `.` nor `:` occurs in a UID or a DNS label, so identifiers split
without ambiguity, and none of them needs escaping in a URL path, query or
fragment. Every number is in canonical decimal (no sign, no leading zero).

## Order and event identity

- `line` is the 1-based ordinal of a line in the attempt's log as the
  server read it from the log's start; `part` numbers the events one line
  produced, from 0 (a Claude result line gives a lifecycle event and a usage
  event). Blank lines produce no event but are counted.
- An event ID is stable for the same log read the same way, so two identical
  lines are two events (different `line`) and a re-read overlap is the same
  event (same `line`). If the start of a log is no longer available
  (rotation), ordinals count from the first line read and a
  `source_truncated` gap says so.
- `seq` orders everything a run's stream delivers: events, attempt updates
  and run updates share one counter, which starts at 1 and increases by
  exactly 1 within an **epoch**. Snapshot events therefore have increasing
  but not necessarily contiguous `seq`s; the SSE stream is contiguous.
- The **epoch** names one server-side history of a run. It changes when that
  history restarts (a server restart, a rebuilt run). `seq`s and positions
  of different epochs are unrelated.

## Event kinds

Every event has `seq`, `id`, `attempt`, `kind`, `source` and exactly the
payload named after its kind. `source.type` is the runner's own event type
(`assistant`, `item.completed`, `tool_use`, ...). `source.logTime` is the
container runtime's receipt time of the line, `source.runnerTime` a time the
runner wrote into it; each is absent when the source had none, and neither
is a duration or an outcome. `truncated` names the payload fields cut to
their limit.

| Kind | Payload | From |
| --- | --- | --- |
| `message` | `role` (`user`, `assistant`), `type` (`text`, `reasoning`), `text`, `messageId?` | text the runner attributed to someone; reasoning only when the runner printed it |
| `tool_call` | `callId`, `name`, `input` or `inputText` (cut serialization), `title?`, `parentCallId?` | a tool invocation |
| `tool_result` | `callId`, `status` (`ok`, `error`, `declined`, `unknown`), `output?`, `exitCode?` | the outcome of the call with the same `callId` |
| `edit` | `path`, `change` (`add`, `update`, `delete`, `unknown`), `callId?`, `diff?`, `before?`, `after?` | a file change, with whatever diff or text the source carried |
| `usage` | `scope` (`message`, `step`, `turn`, `session`) and only the counters reported: tokens, `costUSD` (decimal string), `durationMs`, `apiDurationMs`, `turns` | a runner's usage report; `scope` says what it covers as the runner defines it |
| `error` | `message`, `code?`, `status?` (HTTP), `retry?` (`attempt`, `max?`, `delayMs?`) | an error the runner reported; `retry` only when it announced one |
| `lifecycle` | `phase`, and the fields that phase's source line had | session, turn and Kelos markers (below) |
| `raw` | `reason`, `text` (cut to `maxRawBytes`), `bytes` (the line's length) | a line no adapter rendered |
| `gap` | `reason`, `lines?`, `detail?` | lines missing before `source.line` |

Lifecycle phases: `setup_started`, `setup_completed`, `setup_failed`
(`exitCode`) for the Kelos setup markers on stderr; `session_started`,
`turn_started`, `turn_completed`, `turn_failed`, `compacted`,
`session_completed` (`subtype`, `isError`, `stopReason`) from the runner;
`limit_hit` for the `---KELOS_LIMIT_HIT---` line; `kelos_outputs` for the
`---KELOS_OUTPUTS_START---` block, one `outputs` entry per `key: value`
line, values as written (`response` stays base64).

Raw reasons: `text` (not JSON: startup output, stderr), `malformed_json`
(looks like JSON, does not parse; a cut line), `unknown_type` (JSON of a type
no adapter knows), `unsupported` (a known type the adapter does not render),
`oversized` (longer than `maxLineBytes`; `text` is its start).

Gap reasons: `source_truncated` (the log's start is gone), `read_interrupted`
(the log stream broke and the reconnect could not prove continuity),
`input_limit` (the attempt passed `maxAttemptLogBytes`), `attempt_expired`
(the pod went away while being read).

## States

**Run** `phase` is the Task's: `pending`, `waiting`, `running`, `succeeded`,
`failed`, or `unknown`. `deleted` is set once the Task is seen deleted; the
run keeps its last values. `created`, `started` and `completed` are the
Task's Kubernetes lifecycle times, absent when its status has none.

**Attempt** `container.state` is the Kubernetes container state (`waiting`,
`running`, `terminated`, `unknown`) with its exit code and reason.
`coverage` says how much of the log Lucarne holds:

| `coverage.state` | Meaning |
| --- | --- |
| `pending` | the container has not started, or its log is not read yet |
| `following` | the log is being read as it is written |
| `ended` | the container terminated and its log was read to the end |
| `unavailable` | the log exists but cannot be read now; `reason` says why |
| `expired` | the log is gone (pod deleted, TTL, an older restart) |

`lines` counts lines read, `gaps` the gap events, `truncatedLines` the lines
cut to `maxLineBytes`. An `ended` attempt with no gaps and no truncated lines
is complete as far as Lucarne can tell; Kubernetes gives no stronger
guarantee. Finished Tasks are deleted 72 hours after completion (Kelos
`ttlSecondsAfterFinished`); a pod can disappear sooner.

## Limits

Named in [`limits.go`](limits.go), published in every snapshot and by
`GET /api/v1/contract`, and enforced by the schema where JSON Schema can
(`maxLength` counts characters, an upper bound of the byte limit).

| Limit | Value | Bounds |
| --- | --- | --- |
| `maxLineBytes` | 1 MiB | one log line parsed (input) |
| `maxAttemptLogBytes` | 64 MiB | log read for one attempt (input) |
| `maxTextBytes` | 64 KiB | message text, error message, lifecycle detail, output value |
| `maxToolInputBytes` | 16 KiB | serialized tool input |
| `maxToolOutputBytes` | 32 KiB | tool output |
| `maxDiffBytes` | 64 KiB | edit diff, before and after |
| `maxRawBytes` | 8 KiB | raw text |
| `maxPathBytes` | 4096 | edit path |
| `maxLabelBytes` | 256 | names, IDs, codes, reasons |
| `maxEventsPerRun`, `maxRunBytes` | 20 000, 32 MiB | events a run retains; the oldest go first, counted in `stream.droppedEvents` |
| `maxAttemptsPerRun` | 16 | attempts listed |
| `maxReplayUpdates` | 5 000 | updates replayable to a reconnecting client |
| `maxClientQueue` | 1 024 | updates queued for one slow client |

Text is cut at a UTF-8 boundary. Lifecycle `outputs` holds at most 64
entries and `truncated` at most 8 names.

## Deep links

```
/runs/{cluster}/{namespace}/{taskUID}[?attempt={attemptID}][&view={view}][#event={eventID}]
```

- The path opens the run; it is the only required part. Any other path
  shape, a trailing slash, or a part that is not in canonical form is not a
  run link: the server answers 404 with the UI, which says what is wrong.
- `attempt` selects an attempt; without it the UI follows the latest.
- `view` is `conversation` (the default) or `inspector`; another value is
  invalid. Other query parameters are ignored.
- `#event=` scrolls to an event. An event ID is tied to how a log was read,
  so a stale one (rotation, another epoch) may no longer exist: the UI says
  so rather than guessing a neighbour.
- Links carry no credentials and nothing secret; access control is the
  deployment's (an authenticating proxy), not the link's.

## HTTP API

All endpoints are `GET`, under `/api/v1`, and answer JSON (`no-store`)
except the stream.

| Path | Answer |
| --- | --- |
| `/api/v1/contract` | `ContractInfo`: schema version and limits |
| `/api/v1/runs/{cluster}/{namespace}/{taskUID}` | `Snapshot` |
| `/api/v1/runs/{cluster}/{namespace}/{taskUID}/events` | `text/event-stream` (below) |
| `/healthz` (not versioned) | `ok` |

Errors are an `ErrorResponse` `{"error": {"code", "message"}}`: 400
`invalid_run_key`, 404 `run_not_found` (no such run, or it expired; Lucarne
cannot tell the two apart once the Task is gone), 501 `not_implemented`, 500
`internal`. In this bootstrap the run endpoints validate the key and answer
501.

A **snapshot** is the run, its attempts, its retained events in `seq`
order, and `stream`: the `epoch`, the `lastSeq` it reflects and
`droppedEvents`.

## SSE stream and replay

```
GET /api/v1/runs/{cluster}/{namespace}/{taskUID}/events?after={epoch}:{seq}
Last-Event-ID: {epoch}:{seq}
```

1. The client loads the snapshot, then opens the stream with
   `after={stream.epoch}:{stream.lastSeq}`. On a reconnect the browser sends
   `Last-Event-ID`, which takes precedence over `after`. Without either, the
   stream starts with a `reset`.
2. The server sends every update with `seq` greater than the cursor, then
   new ones as they happen, each once and in order:

   | `event:` | `id:` | `data:` |
   | --- | --- | --- |
   | `transcript` | `{epoch}:{seq}` | `Event` |
   | `attempt` | `{epoch}:{seq}` | `AttemptUpdate` (the attempt's full new state) |
   | `run` | `{epoch}:{seq}` | `RunUpdate` (the run's full new state) |
   | `end` | `{epoch}:{seq}` | `End`: every attempt is ended, expired or unavailable for good, or the run was deleted; the client closes the stream and does not reconnect |
   | `reset` | none | `Reset`: drop all state, load a new snapshot, reconnect from it |

   A comment line (`: keepalive`) is sent when the stream is otherwise idle,
   so proxies keep it open.
3. `reset` reasons: `epoch_changed` (the cursor's epoch is not the current
   one), `replay_expired` (the cursor is older than the last
   `maxReplayUpdates`), `client_too_slow` (more than `maxClientQueue`
   updates were waiting; the server closes the stream after it),
   `bad_cursor` (no cursor, or a malformed one).
4. The client checks that each `seq` is its last plus one. Any other value
   means something was lost: it discards its state and reloads the snapshot,
   as on `reset`.

So a reconnect either resumes exactly or says that it cannot; it never
silently skips or repeats an update. Events lost before the server read them
(source loss) are not a replay problem: they are `gap` events and coverage
counts, delivered like any other update.
