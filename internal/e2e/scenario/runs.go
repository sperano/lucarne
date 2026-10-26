package scenario

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/kube/kubetest"
)

// HostileGlobal is the window property the hostile run's scripts would
// set if the UI ever ran them.
const HostileGlobal = "__lucarneHostile"

// Hostile Markdown in an assistant message: links with unsafe schemes,
// HTML, an image to a remote host.
var HostileMarkdown = []string{
	"[run me](javascript:window." + HostileGlobal + "=1)",
	"<script>window." + HostileGlobal + "=1</script>",
	`<img src="x" onerror="window.` + HostileGlobal + `=1">`,
	"[as data](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
	"[vb](vbscript:msgbox(1))",
	"![pixel](https://tracker.example.com/pixel.png)",
	"[safe](https://docs.example.com/page)",
}

// HostileRaw are lines no adapter parses: broken, deep, odd or unsafe JSON
// and text.
var HostileRaw = []string{
	`{"type":"assistant","message":`,
	`{"__proto__":{"polluted":true},"constructor":{"prototype":{"polluted":true}},"type":"system","subtype":"init"}`,
	strings.Repeat("[", hostileDepth) + strings.Repeat("]", hostileDepth),
	`{"type":"<img src=x onerror=window.` + HostileGlobal + `=1>"}`,
	"\x1b[31mred\x1b[0m and a NUL \x00 byte",
	"invalid UTF-8: \xff\xfe",
	`{"type":"result","total_cost_usd":1e999}`,
	"<script>window." + HostileGlobal + "=1</script>",
}

// hostileDepth is how deeply the hostile JSON line nests.
const hostileDepth = 10000

// Sizes of the oversized output.
const (
	// LongLineBytes is a line longer than the adapters parse.
	LongLineBytes = contract.MaxLineBytes + 4096
	// LongTextBytes is a message longer than an event holds.
	LongTextBytes = contract.MaxTextBytes * 2
	// LongToolOutputBytes is a tool output longer than an event holds.
	LongToolOutputBytes = contract.MaxToolOutputBytes * 2
	// LongSessionMessages is more messages than a run retains.
	LongSessionMessages = contract.MaxEventsPerRun + 5000
)

// ForeignMarker is in the log of the pod Lucarne must never read.
const ForeignMarker = "FOREIGN-POD-LINE"

// PrivateMarker is in the log of the run outside the allowlist.
const PrivateMarker = "PRIVATE-NAMESPACE-LINE"

// sessionID is the fabricated Claude Code session of the generated runs.
const sessionID = "0b1c2d3e-4f50-4617-8293-a4b5c6d7e8f9"

func (w *World) addHostile() error {
	lines := []string{
		claudeInit(),
		claudeText(1, "Hostile Markdown follows.\n\n"+strings.Join(HostileMarkdown, "\n\n")),
		claudeToolUse(2, "toolu_hostile", "Bash", map[string]any{"command": "echo '<script>window." + HostileGlobal + "=1</script>'"}),
		claudeToolResult("toolu_hostile", "</pre><script>window."+HostileGlobal+"=1</script>"),
	}
	lines = append(lines, HostileRaw...)
	return w.addFinished(Hostile, string(contract.RunnerClaudeCode), lines, false)
}

func (w *World) addLong() error {
	long := []string{
		claudeInit(),
		claudeText(1, "A line longer than the adapters parse follows."),
		claudeText(2, strings.Repeat("x", LongLineBytes)),
		claudeText(3, strings.Repeat("y", LongTextBytes)),
		claudeToolUse(4, "toolu_long", "Bash", map[string]any{"command": "cat big.txt"}),
		claudeToolResult("toolu_long", strings.Repeat("z", LongToolOutputBytes)),
		claudeText(5, "Done after the long output."),
	}
	if err := w.addFinished(LongLine, string(contract.RunnerClaudeCode), long, false); err != nil {
		return err
	}
	session := make([]string, 0, LongSessionMessages+1)
	session = append(session, claudeInit())
	for i := 1; i <= LongSessionMessages; i++ {
		session = append(session, claudeText(i, fmt.Sprintf("Message %d of a long session.", i)))
	}
	return w.addFinished(LongSession, string(contract.RunnerClaudeCode), session, false)
}

// addUnread adds what the server must never read: a run outside the
// allowlist, and a pod labelled for a run that its Job does not own.
func (w *World) addUnread() error {
	private, err := w.add(PrivateRun, PrivateNamespace, string(contract.RunnerClaudeCode), kubetest.PhaseRunning)
	if err != nil {
		return err
	}
	private.Pod.Write(claudeInit(), claudeText(1, PrivateMarker))
	if err := w.addFinished(ForeignPod, string(contract.RunnerClaudeCode), []string{claudeInit(), claudeText(1, "The run's own line.")}, false); err != nil {
		return err
	}
	foreign, err := w.Cluster.AddForeignPod(Namespace, taskName(ForeignPod)+"-zzzzz", newUID(), taskName(ForeignPod))
	if err != nil {
		return err
	}
	foreign.Write(claudeInit(), claudeText(1, ForeignMarker))
	return nil
}

// addOthers adds a run whose pod is gone and one of a runner Lucarne has
// no adapter for.
func (w *World) addOthers() error {
	if err := w.addFinished(PodDeleted, string(contract.RunnerClaudeCode), []string{claudeInit(), claudeText(1, "Lost with the pod.")}, false); err != nil {
		return err
	}
	deleted := w.Run(PodDeleted)
	if err := deleted.Pod.Delete(); err != nil {
		return err
	}
	deleted.Pod = nil
	return w.addFinished(UnknownRunner, "gemini", []string{`{"type":"gemini","text":"kept raw"}`, "plain text"}, false)
}

// ClaudeText is a Claude Code assistant message holding text, as the n-th
// message of a session.
func ClaudeText(n int, text string) string { return claudeText(n, text) }

func claudeInit() string {
	return mustJSON(map[string]any{
		"type": "system", "subtype": "init", "cwd": "/workspace/repo", "session_id": sessionID,
		"tools": []string{"Bash", "Read"}, "model": "claude-opus-5-5", "claude_code_version": "2.1.296",
		"uuid": "00000000-0000-4000-8000-000000000000",
	})
}

func claudeText(n int, text string) string {
	return claudeAssistant(n, map[string]any{"type": "text", "text": text})
}

func claudeToolUse(n int, id, name string, input map[string]any) string {
	return claudeAssistant(n, map[string]any{"type": "tool_use", "id": id, "name": name, "input": input})
}

func claudeAssistant(n int, content map[string]any) string {
	return mustJSON(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "claude-opus-5-5", "id": fmt.Sprintf("msg_%08d", n), "type": "message", "role": "assistant",
			"content": []any{content}, "stop_reason": nil,
		},
		"parent_tool_use_id": nil, "session_id": sessionID, "uuid": messageUUID(n),
	})
}

func claudeToolResult(id, output string) string {
	return mustJSON(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"tool_use_id": id, "type": "tool_result", "content": output, "is_error": false}},
		},
		"parent_tool_use_id": nil, "session_id": sessionID,
	})
}

// messageUUID is the fabricated UUID of the n-th message.
func messageUUID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

// mustJSON encodes v, which holds only strings, numbers, maps and slices.
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("scenario: encode a line: %v", err))
	}
	return string(data)
}
