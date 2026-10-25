package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Tools whose structured output describes a file change.
const (
	toolEdit      = "Edit"
	toolMultiEdit = "MultiEdit"
	toolWrite     = "Write"
)

// Write's structured output type.
const (
	writeCreate = "create"
	writeUpdate = "update"
)

// diffLineEnd ends every line of a rendered diff.
const diffLineEnd = "\n"

// fileChange is the structured output (tool_use_result) of Edit, MultiEdit
// and Write. structuredPatch holds the jsdiff hunks of the change.
type fileChange struct {
	Type            string  `json:"type"`
	FilePath        string  `json:"filePath"`
	Content         *string `json:"content"`
	StructuredPatch []hunk  `json:"structuredPatch"`
}

type hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// editEvent is the file change a successful call of tool made, from its
// structured output. A call whose tool is unknown (its tool_use came
// before the log read, or was evicted) is judged by the output's shape.
func editEvent(tool, callID string, structured json.RawMessage) (contract.Event, bool) {
	var change fileChange
	if !isObject(structured) || json.Unmarshal(structured, &change) != nil || change.FilePath == "" {
		return contract.Event{}, false
	}
	kind, ok := changeKind(tool, change)
	if !ok {
		return contract.Event{}, false
	}
	var cut adapter.Truncation
	edit := &contract.Edit{
		CallID: callID,
		Path:   cut.Text("path", change.FilePath, contract.MaxPathBytes),
		Change: kind,
	}
	if change.StructuredPatch != nil && kind != contract.ChangeAdd {
		diff := unifiedDiff(change.StructuredPatch)
		edit.Diff = cut.Optional("diff", &diff, contract.MaxDiffBytes)
	}
	if tool == toolWrite || (tool == "" && isWriteType(change.Type)) {
		edit.After = cut.Optional("after", change.Content, contract.MaxDiffBytes)
	}
	return contract.Event{Kind: contract.KindEdit, Edit: edit, Truncated: cut.Fields()}, true
}

// changeKind says what the call did to its file, and false when the call
// is not a file change.
func changeKind(tool string, change fileChange) (contract.EditChange, bool) {
	switch tool {
	case toolEdit, toolMultiEdit:
		return contract.ChangeUpdate, true
	case toolWrite:
		return writeChange(change.Type), true
	case "":
		if change.StructuredPatch == nil {
			return "", false
		}
		if isWriteType(change.Type) {
			return writeChange(change.Type), true
		}
		return contract.ChangeUpdate, true
	}
	return "", false
}

func writeChange(writeType string) contract.EditChange {
	switch writeType {
	case writeCreate:
		return contract.ChangeAdd
	case writeUpdate:
		return contract.ChangeUpdate
	}
	return contract.ChangeUnknown
}

func isWriteType(t string) bool { return t == writeCreate || t == writeUpdate }

// unifiedDiff renders jsdiff hunks as a unified diff body, as jsdiff's
// formatPatch writes them, without the file header.
func unifiedDiff(hunks []hunk) string {
	var b strings.Builder
	for _, h := range hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s", h.OldStart, h.OldLines, h.NewStart, h.NewLines, diffLineEnd)
		for _, line := range h.Lines {
			b.WriteString(line)
			b.WriteString(diffLineEnd)
		}
	}
	return b.String()
}

func isObject(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '{'
}
