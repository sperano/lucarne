package claude

import (
	"encoding/json"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// System subtypes the parser renders as runner notices: hooks, statuses and
// messages the CLI prints around the conversation.
const (
	subtypeHookStarted            = "hook_started"
	subtypeHookProgress           = "hook_progress"
	subtypeHookResponse           = "hook_response"
	subtypeStatus                 = "status"
	subtypeInformational          = "informational"
	subtypeNotification           = "notification"
	subtypeLocalCommandOutput     = "local_command_output"
	subtypeMemoryRecall           = "memory_recall"
	subtypeFilesPersisted         = "files_persisted"
	subtypePluginInstall          = "plugin_install"
	subtypeSessionStateChanged    = "session_state_changed"
	subtypeBackgroundTasksChanged = "background_tasks_changed"
	subtypeModelRefusalFallback   = "model_refusal_fallback"
	subtypeModelRefusalNoFallback = "model_refusal_no_fallback"
	subtypeMirrorError            = "mirror_error"
	subtypePermissionDenied       = "permission_denied"
	subtypeWorkerShuttingDown     = "worker_shutting_down"
)

// Line types the parser renders as runner notices.
const (
	typeToolUseSummary = "tool_use_summary"
	typeAuthStatus     = "auth_status"
)

// Values of the notice fields the parser tells apart.
const (
	hookOutcomeSuccess   = "success"
	hookOutcomeError     = "error"
	compactResultFailed  = "failed"
	pluginInstallFailed  = "failed"
	noticeFactSeparator  = " · "
	noticeParagraphBreak = "\n\n"
)

// notice is what a runner notice line says: its text, a model, an exit code
// and whether it reports something going wrong.
type notice struct {
	detail   string
	model    string
	exitCode *int
	isError  bool
}

// noticeReader reads one kind of notice line; it reads a line without the
// shape of its kind as a notice with no detail: still a well-formed line of
// a kind the runner documents.
type noticeReader func(text []byte) notice

// systemNotices are the system subtypes rendered as runner notices.
var systemNotices = map[string]noticeReader{
	subtypeHookStarted:            readHook,
	subtypeHookProgress:           readHook,
	subtypeHookResponse:           readHook,
	subtypeStatus:                 readStatus,
	subtypeInformational:          readField(func(l textFields) string { return l.Content }),
	subtypeNotification:           readField(func(l textFields) string { return l.Text }),
	subtypeLocalCommandOutput:     readField(func(l textFields) string { return l.Content }),
	subtypeMemoryRecall:           readMemoryRecall,
	subtypeFilesPersisted:         readFilesPersisted,
	subtypePluginInstall:          readPluginInstall,
	subtypeSessionStateChanged:    readField(func(l textFields) string { return l.State }),
	subtypeBackgroundTasksChanged: readBackgroundTasks,
	subtypeModelRefusalFallback:   readRefusal,
	subtypeModelRefusalNoFallback: readRefusal,
	subtypeMirrorError:            readError(func(l textFields) string { return l.Error }),
	subtypePermissionDenied:       readPermissionDenied,
	subtypeWorkerShuttingDown:     readField(func(l textFields) string { return l.Reason }),
}

// typeNotices are the line types rendered as runner notices.
var typeNotices = map[string]noticeReader{
	typeToolUseSummary: readField(func(l textFields) string { return l.Summary }),
	typeAuthStatus:     readAuthStatus,
}

// renderNotice adds the runner notice of a line whose kind is name.
func renderNotice(name string, read noticeReader, text []byte, out *adapter.Emitter) bool {
	out.Emit(noticeEvent(name, read(text)))
	return true
}

// noticeEvent is a runner_notice lifecycle event named after the kind of
// line it comes from.
func noticeEvent(name string, n notice) contract.Event {
	var cut adapter.Truncation
	ev := lifecycle(contract.LifecycleRunnerNotice)
	ev.Lifecycle.Subtype = cut.Text("subtype", name, contract.MaxLabelBytes)
	ev.Lifecycle.Model = cut.Text("model", n.model, contract.MaxLabelBytes)
	ev.Lifecycle.ExitCode = n.exitCode
	if n.detail != "" {
		ev.Lifecycle.Detail = cut.Optional("detail", &n.detail, contract.MaxTextBytes)
	}
	if n.isError {
		ev.Lifecycle.IsError = &n.isError
	}
	ev.Truncated = cut.Fields()
	return ev
}

// kindName is the name of a line's kind: a system line's subtype, else its
// type.
func kindName(h head) string {
	if h.Type == typeSystem && h.Subtype != "" {
		return h.Subtype
	}
	return h.Type
}

// textFields holds the single-text fields of the notice kinds that carry
// one.
type textFields struct {
	Content string `json:"content"`
	Text    string `json:"text"`
	State   string `json:"state"`
	Summary string `json:"summary"`
	Error   string `json:"error"`
	Reason  string `json:"reason"`
}

// readField reads a notice whose detail is one text field.
func readField(field func(textFields) string) noticeReader {
	return func(text []byte) notice {
		var l textFields
		if json.Unmarshal(text, &l) != nil {
			return notice{}
		}
		return notice{detail: field(l)}
	}
}

// readError reads a notice that reports an error, whose detail is one text
// field.
func readError(field func(textFields) string) noticeReader {
	read := readField(field)
	return func(text []byte) notice {
		n := read(text)
		n.isError = true
		return n
	}
}

type hookLine struct {
	HookName  string `json:"hook_name"`
	HookEvent string `json:"hook_event"`
	Output    string `json:"output"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  *int   `json:"exit_code"`
	Outcome   string `json:"outcome"`
}

// readHook reads hook_started, hook_progress and hook_response: the hook's
// name (its event when it has none) and an outcome other than success, then
// its output, or its stdout and stderr when the line has no combined output.
func readHook(text []byte) notice {
	var l hookLine
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	name := l.HookName
	if name == "" {
		name = l.HookEvent
	}
	outcome := l.Outcome
	if outcome == hookOutcomeSuccess {
		outcome = ""
	}
	output := l.Output
	if output == "" {
		output = paragraphs(l.Stdout, l.Stderr)
	}
	return notice{
		detail:   paragraphs(facts(name, outcome), output),
		exitCode: l.ExitCode,
		isError:  l.Outcome == hookOutcomeError,
	}
}

type statusLine struct {
	Status        *string `json:"status"`
	CompactResult string  `json:"compact_result"`
	CompactError  string  `json:"compact_error"`
}

// readStatus reads a status line: the status (compacting, requesting; null
// when it clears), the result of a compaction and its error.
func readStatus(text []byte) notice {
	var l statusLine
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	status := ""
	if l.Status != nil {
		status = *l.Status
	}
	result := ""
	if l.CompactResult != "" {
		result = "compact " + l.CompactResult
	}
	return notice{
		detail:  paragraphs(facts(status, result), l.CompactError),
		isError: l.CompactResult == compactResultFailed,
	}
}

// readMemoryRecall reads the paths of the memories recalled, one per line.
func readMemoryRecall(text []byte) notice {
	var l struct {
		Memories []struct {
			Path string `json:"path"`
		} `json:"memories"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	paths := make([]string, 0, len(l.Memories))
	for _, m := range l.Memories {
		paths = append(paths, m.Path)
	}
	return notice{detail: lines(paths...)}
}

// readFilesPersisted reads the files saved, one per line, then those that
// failed with their error.
func readFilesPersisted(text []byte) notice {
	var l struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
		Failed []struct {
			Filename string `json:"filename"`
			Error    string `json:"error"`
		} `json:"failed"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	saved := make([]string, 0, len(l.Files))
	for _, f := range l.Files {
		saved = append(saved, f.Filename)
	}
	failed := make([]string, 0, len(l.Failed))
	for _, f := range l.Failed {
		failed = append(failed, f.Filename+": "+f.Error)
	}
	return notice{detail: paragraphs(lines(saved...), lines(failed...)), isError: len(failed) > 0}
}

// readPluginInstall reads the plugin's name, the install's status and its
// error.
func readPluginInstall(text []byte) notice {
	var l struct {
		Status string `json:"status"`
		Name   string `json:"name"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	return notice{
		detail:  paragraphs(facts(l.Name, l.Status), l.Error),
		isError: l.Status == pluginInstallFailed,
	}
}

// readBackgroundTasks reads the descriptions of the background tasks still
// running, one per line.
func readBackgroundTasks(text []byte) notice {
	var l struct {
		Tasks []struct {
			Description string `json:"description"`
		} `json:"tasks"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	descriptions := make([]string, 0, len(l.Tasks))
	for _, t := range l.Tasks {
		descriptions = append(descriptions, t.Description)
	}
	return notice{detail: lines(descriptions...)}
}

// readRefusal reads a model refusal: the CLI's text, and the model it fell
// back to (the refusing one when it has no fallback).
func readRefusal(text []byte) notice {
	var l struct {
		Content       string `json:"content"`
		OriginalModel string `json:"original_model"`
		FallbackModel string `json:"fallback_model"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	model := l.FallbackModel
	if model == "" {
		model = l.OriginalModel
	}
	return notice{detail: l.Content, model: model, isError: true}
}

// readPermissionDenied reads the tool denied and the CLI's message.
func readPermissionDenied(text []byte) notice {
	var l struct {
		ToolName string `json:"tool_name"`
		Message  string `json:"message"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	return notice{detail: paragraphs(l.ToolName, l.Message), isError: true}
}

// readAuthStatus reads an authentication status: its error, then its
// output lines.
func readAuthStatus(text []byte) notice {
	var l struct {
		Output []string `json:"output"`
		Error  string   `json:"error"`
	}
	if json.Unmarshal(text, &l) != nil {
		return notice{}
	}
	return notice{detail: paragraphs(l.Error, lines(l.Output...)), isError: l.Error != ""}
}

// facts joins the values that are not empty on one line.
func facts(values ...string) string { return join(noticeFactSeparator, values) }

// lines joins the values that are not empty one per line.
func lines(values ...string) string { return join("\n", values) }

// paragraphs joins the texts that are not empty with a blank line between.
func paragraphs(texts ...string) string { return join(noticeParagraphBreak, texts) }

func join(sep string, values []string) string {
	kept := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			kept = append(kept, v)
		}
	}
	return strings.Join(kept, sep)
}
