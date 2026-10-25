// Package codex normalizes the output of codex exec --json (as Kelos starts
// it, Codex 0.162.1) into transcript events. It is an adapter.Runner: it
// sees every line that is not blank, oversized or a Kelos marker, and keeps
// a digest of what it last rendered of each recent item.
//
// What each line becomes:
//
//   - thread.started: a session_started lifecycle event whose session is
//     the thread ID. Codex prints neither the model nor its version.
//   - turn.started: a turn_started lifecycle event.
//   - turn.completed: a turn_completed lifecycle event, then the usage as
//     a session-scoped usage event: it is the thread's running total, not
//     the turn's. input counts the cached input too, as Codex counts it
//     (cached_input_tokens is cacheRead, cache_write_input_tokens
//     cacheWrite, reasoning_output_tokens reasoning). Codex prints all
//     zeros when it received no count; that is no usage event.
//   - turn.failed: a turn_failed lifecycle event whose detail is the
//     error's message. A failure from the stream printed the same message
//     as an error line just before, which is the error event.
//   - error: an error event. "Reconnecting... n/max" announces a stream
//     retry, its attempt and max.
//   - item.started, item.updated, item.completed: by item type, below. An
//     item is correlated by its ID across its lines, and each change is
//     rendered once: a line that repeats what its item showed already
//     produces no event.
//   - anything else is raw: a type the parser does not know as
//     unknown_type, a known type of an unexpected shape (an item without
//     an ID, a command without a command) as unsupported, a line that does
//     not parse as malformed_json, and text (startup output, stderr) as
//     text.
//
// Items:
//
//   - agent_message and reasoning: an assistant message, text or
//     reasoning, whose messageId is the item ID. Codex prints both only as
//     item.completed.
//   - error: an error event. Codex makes warnings, configuration and
//     deprecation notices and model reroutes error items.
//   - command_execution, file_change, mcp_tool_call, collab_tool_call,
//     web_search and todo_list: a tool call named after the item type,
//     with the item ID as its callId and, as its input, the item's fields
//     but id, type, status and those of the outcome. The call is rendered
//     from the first line of the item, again when its input changes (a
//     todo list's items: a later tool_call of the same callId supersedes
//     the earlier), and item.completed adds the result: ok for completed,
//     error for failed, declined for declined, unknown when the item has
//     no status (web_search, todo_list) or completed in_progress (a turn
//     interrupted before the call finished). A call the log ends before
//     has no result.
//
// What a tool result holds is exactly what Codex prints on stdout:
//
//   - command_execution: the command line as Codex ran it (with its shell
//     wrapper), aggregated_output (stdout and stderr in one string, only
//     on item.completed: no output is printed while the command runs) and
//     exit_code. Codex prints "" for an output it has none of, so an empty
//     output without an exit code (a declined or unfinished command) is
//     none. Files the agent reads reach the log only as the output of the
//     command that read them.
//   - file_change: each change's path and kind (add, delete, update) as an
//     edit, after a completed change only; no diff, no content, no
//     destination of a move. A declined patch is failed, an error result,
//     as Codex reports it.
//   - mcp_tool_call: server, tool and arguments as the input; the error's
//     message, else the result's text when all its content is text, else
//     the result (content and structured_content) as JSON.
//   - collab_tool_call: agents_states as JSON; Codex omits some collab
//     tools and interrupted calls entirely.
//   - web_search: query and action as the input, results as JSON when
//     printed. The item serializes "id" twice; the first, the item ID, is
//     kept and the search's own ID is not rendered.
//   - todo_list: the items as the input, no output; it completes when the
//     turn ends.
//
// Capability gaps of Codex 0.162.1 as Kelos runs it, and of this adapter:
//
//   - Not printed: the prompt and any user message, hidden reasoning (only
//     the summary is an item), costs, per-turn usage, the model, times, the
//     unified diff of a turn, partial messages and command output deltas.
//     Hooks and model verifications print nothing.
//   - An interrupted turn prints no turn line; Codex completes the items it
//     started with the state the turn holds, which this adapter shows as
//     an unknown result (the status Codex then prints is unverified).
//   - Repeats are found by digest over the last maxSeenItems items; an
//     older item renders its next line in full again.
package codex
