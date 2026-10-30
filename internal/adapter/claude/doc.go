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
//   - system/task_started, tool_progress, system/task_progress and
//     system/task_notification: tool progress events correlated to the
//     monitored call. They carry the start, elapsed time and completion
//     status the source reports. A task_progress line (a subagent's running
//     report) is running, its usage.duration_ms the elapsed time, its
//     description the detail and its subagent_type the subtype.
//   - rate_limit_event: a rate_limit_status lifecycle event whose subtype is
//     the status, whose sessionId is the source session and whose detail is
//     rate_limit_info as written.
//   - assistant: one event per content block, in order: text and thinking
//     become messages carrying the API message ID, tool_use a tool call
//     (parentCallId is the Task call of a subagent's line). A line with an
//     error field (an API error the CLI printed as a message, such as
//     rate_limit) gives error events instead of text messages. The message's
//     usage is emitted only from the line that carries its stop_reason.
//   - user: text becomes a user message, whose media are the image and
//     document blocks right after it (blocks no text comes right before get
//     a message of their own, without text); each tool_result a tool result
//     correlated by tool_use_id, ok unless is_error is true (the API's
//     default), whose output is its text blocks and the names of the tools
//     its tool_reference blocks load (a tool search's), one per line, and
//     whose media are its image and document blocks. A media entry is the
//     block's type, the source's media_type, the decoded size of a base64
//     source and a document's title, never the bytes; a media block without
//     a source of a type the API defines for it stays raw. A successful
//     Edit, MultiEdit or Write also gives an edit from tool_use_result: the
//     jsdiff hunks as a unified diff, and Write's new content as after.
//   - result: a session_completed lifecycle event, the session's usage
//     (tokens, total_cost_usd as a decimal string, durations, turns), and an
//     error per errors entry.
//   - anything else is raw: stream_event (partial messages, which Kelos does
//     not ask for) as unsupported, other types and system subtypes as
//     unknown_type, a line that does not parse as malformed_json, and text as
//     text. A content block the parser does not render is raw too, as its own
//     event of the line.
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
//     redacted thinking, the bytes of images and documents, the size of a
//     media source other than base64 (the log does not give it), images and
//     documents of assistant messages, and the edits of NotebookEdit
//     or of tools other than Edit, MultiEdit and Write. Background monitoring
//     is correlated through tool_use_id; its runner task/run IDs, output
//     file, and a subagent's token and tool-use counts and last tool remain
//     available in the source line rather than the normalized event.
//   - Times: a line's timestamp field, when present, is its runnerTime; the
//     fixtures carry none, so which lines have one is unverified.
package claude
