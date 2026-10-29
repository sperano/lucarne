// Package claude normalizes Claude Code's stream-json output (claude -p
// --output-format stream-json --verbose, as Kelos starts it) into transcript
// events. It is an adapter.Runner: it sees every line that is not blank,
// oversized or a Kelos marker, and keeps only the IDs of tool calls waiting
// for their result.
//
// What each line becomes:
//
//   - system/init: a session_started lifecycle event (session, model, CLI
//     version).
//   - system/api_retry: an error event with the HTTP status (absent when no
//     response came) and the announced retry.
//   - system/compact_boundary: a compacted lifecycle event whose detail is
//     compact_metadata as written.
//   - system/thinking_tokens: a thinking lifecycle event (session), one per
//     progress line while a thinking block streams.
//   - rate_limit_event: a rate_limit_status lifecycle event whose subtype is
//     the status, whose sessionId is the source session and whose detail is
//     rate_limit_info as written.
//   - assistant: one event per content block, in order: text and thinking
//     become messages carrying the API message ID, tool_use a tool call
//     (parentCallId is the Task call of a subagent's line). A line with an
//     error field (an API error the CLI printed as a message, such as
//     rate_limit) gives error events instead of text messages. The message's
//     usage is emitted only from the line that carries its stop_reason.
//   - user: text becomes a user message; each tool_result a tool result
//     correlated by tool_use_id, ok unless is_error is true (the API's
//     default); a successful Edit, MultiEdit or Write also gives an edit from
//     tool_use_result: the jsdiff hunks as a unified diff, and Write's new
//     content as after.
//   - result: a session_completed lifecycle event, the session's usage
//     (tokens, total_cost_usd as a decimal string, durations, turns), and an
//     error per errors entry.
//   - anything else is raw: stream_event (partial messages, which Kelos does
//     not ask for), tool_progress (a heartbeat while a tool call runs) and
//     the system subtypes task_started and task_notification (the CLI's
//     background monitoring of a tool call, carried already by its own
//     tool_use/tool_result) as unsupported, other types and system subtypes
//     as unknown_type, a line that does not parse as malformed_json, and
//     text as text. A content block the parser does not render is raw too,
//     as its own event of the line.
//
// Capability gaps of Claude Code 2.1.296 as Kelos runs it, and of this
// adapter:
//
//   - Partial messages: every content block arrives whole, on its own line;
//     the lines of one message share its ID and are not merged. The usage on
//     a line without a stop_reason is the count at the message's start, so
//     it is not reported; real runs may print no stop_reason on lines that
//     end in tool_use, leaving only the session's usage.
//   - Tool status: no exit code (Bash reports it only inside the error
//     text), and no declined or cancelled status: an interrupted, denied or
//     cancelled call is an error result with the CLI's text, and a call the
//     run ended before has no result at all.
//   - Messages and results inside a subagent are not marked as such: the
//     contract has a parent only on tool calls.
//   - Not rendered: the estimated token count of thinking progress
//     (thinking_tokens), the per-model usage (modelUsage), permission_denials,
//     the result text (it repeats the last assistant message), retry waits without a response (no_response),
//     redacted thinking, images and documents, the edits of NotebookEdit
//     or of tools other than Edit, MultiEdit and Write, and the background
//     monitoring of a running tool call (task_started, tool_progress
//     heartbeats, task_notification): its task_id and run_id are not
//     correlated with anything else, and its description, elapsed time and
//     summary duplicate what the tool_use and tool_result already carry.
//   - Times: a line's timestamp field, when present, is its runnerTime; the
//     fixtures carry none, so which lines have one is unverified.
package claude
