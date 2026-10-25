// Package opencode normalizes OpenCode's run --format json output (opencode
// run --format json --auto, as Kelos starts it) into transcript events. It
// is an adapter.Runner: it sees every line that is not blank, oversized or a
// Kelos marker, and keeps no state between lines.
//
// Every line OpenCode 1.19.0 prints is {type, timestamp, sessionID, ...}
// (packages/opencode/src/cli/cmd/run.ts, emit), timestamp being when the
// line was written, in milliseconds; it is the events' runnerTime. What each
// type becomes:
//
//   - step_start: a step_started lifecycle event with the session ID. A step
//     is one model call; OpenCode prints no session or turn start.
//   - step_finish: a step_completed lifecycle event whose stopReason is the
//     finish reason (stop, tool-calls, length, ...), then the step's usage:
//     input, output, reasoning, cache read and write and total tokens, and
//     the cost as a decimal string.
//   - text: a message carrying the part's messageID. run.ts prints a text
//     part only once it has ended, so it is whole. A synthetic part (text
//     OpenCode writes into a user message, such as the prompt after a
//     compaction) is the user's; any other is the assistant's.
//   - reasoning: an assistant reasoning message (printed only with
//     --thinking, which Kelos does not pass).
//   - tool_use: a tool part, printed once completed or failed, so one line
//     holds the call and its outcome: a tool call (input, and the title of a
//     completed call), then a tool result: ok with the output when
//     completed, error with OpenCode's error text when failed. The bash
//     tool's metadata.exit is the exit code. A completed call's attachments
//     follow as a raw unsupported event, then the file changes of edit,
//     write and apply_patch as edits.
//   - error: an error event: a named error's data.message (its name when it
//     has none), the name as code and APIError's statusCode as status; any
//     other error shape keeps its JSON as the message.
//   - anything else is raw: other types as unknown_type, a known type of an
//     unexpected shape as unsupported, a line that does not parse (a cut
//     line) as malformed_json, and text (the entrypoint's warnings, stderr)
//     as text.
//
// Edits: edit's metadata.diff without its file header, an add when its
// oldString is empty (the tool then creates the file) and an update
// otherwise; write's content as after, an add or an update by
// metadata.exists, unknown without it; apply_patch's metadata.files, one
// edit each, a move as a delete of the old path and an add of the new one
// whose diff is relative to the old content.
//
// Capability gaps of OpenCode 1.19.0 as Kelos runs it, and of this adapter:
//
//   - No session start or end, no model, no runner version, no user prompt
//     (the prompt's text part never ends), no session usage: only steps and
//     their usage.
//   - No retry is printed: the processor's retry parts and session.status
//     retry never reach stdout, only a final error. APIError's isRetryable
//     says the provider call may be retried, not that OpenCode will.
//   - No partial output: text, reasoning and tool parts are printed once
//     finished, never while they stream. A tool still running when the
//     run is aborted is printed as failed; one still running when the
//     process is killed prints nothing.
//   - Tool status: a command that exits non-zero is completed, so its
//     result is ok with the exit code; error is a tool that threw, a denied
//     permission or question, or an aborted call ("Tool execution aborted").
//     A denial is not told apart from other errors, and no declined status
//     exists (--auto approves every permission).
//   - Subagents: run.ts prints only the parts of the run's own session, so a
//     task call's subagent is its call and result only.
//   - Repeated parts: a completed tool part is printed again when compaction
//     prunes its output (state.time.compacted); each print is its own call
//     and result events.
//   - Not rendered: a failed call's metadata (the output an aborted command
//     had streamed, interrupted), a completed call's other metadata, the
//     step snapshots, and the time and ID of each part.
//   - Edits: no before text; write's after is the content asked for, which a
//     formatter may have changed; a file changed by bash is not an edit.
package opencode
