package opencode

import (
	"encoding/json"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Tools whose completed state describes a file change (packages/opencode/
// src/tool).
const (
	toolEdit       = "edit"
	toolWrite      = "write"
	toolApplyPatch = "apply_patch"
)

// editTools are the tools edits reads.
var editTools = map[string]bool{toolEdit: true, toolWrite: true, toolApplyPatch: true}

// apply_patch file change types.
const (
	patchAdd    = "add"
	patchUpdate = "update"
	patchDelete = "delete"
	patchMove   = "move"
)

// hunkStart starts a hunk of a unified diff; what comes before the first
// one is the file header createTwoFilesPatch writes, which starts with
// headerStart.
const (
	hunkStart   = "@@"
	headerStart = "Index: "
)

// fileInput is the input of edit and write.
type fileInput struct {
	FilePath  string  `json:"filePath"`
	OldString *string `json:"oldString"`
	Content   *string `json:"content"`
}

// editMetadata is the completed metadata of edit, write and apply_patch.
type editMetadata struct {
	// Diff is edit's patch of the file; FileDiff names the file.
	Diff     *string `json:"diff"`
	FileDiff *struct {
		File string `json:"file"`
	} `json:"filediff"`
	// FilePath and Exists are write's: the file, and whether it existed.
	FilePath string `json:"filepath"`
	Exists   *bool  `json:"exists"`
	// Files are apply_patch's, one per file it changed.
	Files []patchFile `json:"files"`
}

type patchFile struct {
	FilePath string  `json:"filePath"`
	Type     string  `json:"type"`
	Patch    *string `json:"patch"`
	MovePath string  `json:"movePath"`
}

// edits are the file changes a completed call of tool made, from its input
// and metadata; none for another tool or a shape it did not expect.
func edits(tool, callID string, state *toolState) []contract.Event {
	var meta editMetadata
	if !editTools[tool] || !isObject(state.Metadata) || json.Unmarshal(state.Metadata, &meta) != nil {
		return nil
	}
	var input fileInput
	if isObject(state.Input) && json.Unmarshal(state.Input, &input) != nil {
		input = fileInput{}
	}
	switch tool {
	case toolEdit:
		return single(editChange(callID, input, meta))
	case toolWrite:
		return single(writeChange(callID, input, meta))
	case toolApplyPatch:
		return patchChanges(callID, meta.Files)
	}
	return nil
}

func single(ev contract.Event, ok bool) []contract.Event {
	if !ok {
		return nil
	}
	return []contract.Event{ev}
}

// editChange is edit's change: an empty oldString creates the file (the
// tool refuses it for a file that exists), any other replaces text in it.
func editChange(callID string, input fileInput, meta editMetadata) (contract.Event, bool) {
	path := input.FilePath
	if meta.FileDiff != nil && meta.FileDiff.File != "" {
		path = meta.FileDiff.File
	}
	if path == "" || input.OldString == nil {
		return contract.Event{}, false
	}
	change := contract.ChangeUpdate
	if *input.OldString == "" {
		change = contract.ChangeAdd
	}
	return editEvent(callID, path, change, hunks(meta.Diff), nil), true
}

// writeChange is write's change: whether the file existed says which, and
// the content written is the after text. A formatter OpenCode runs after
// the write may have changed the file; the source does not say.
func writeChange(callID string, input fileInput, meta editMetadata) (contract.Event, bool) {
	path := meta.FilePath
	if path == "" {
		path = input.FilePath
	}
	if path == "" {
		return contract.Event{}, false
	}
	change := contract.ChangeUnknown
	if meta.Exists != nil {
		change = contract.ChangeAdd
		if *meta.Exists {
			change = contract.ChangeUpdate
		}
	}
	return editEvent(callID, path, change, nil, input.Content), true
}

// patchChanges are apply_patch's changes, one per file. A move writes the
// new path and removes the old one, so it is a delete of the old path and
// an add of the new one carrying the patch, which is relative to the old
// file's content.
func patchChanges(callID string, files []patchFile) []contract.Event {
	var events []contract.Event
	for _, f := range files {
		if f.FilePath == "" {
			continue
		}
		diff := hunks(f.Patch)
		switch f.Type {
		case patchAdd:
			events = append(events, editEvent(callID, f.FilePath, contract.ChangeAdd, diff, nil))
		case patchUpdate:
			events = append(events, editEvent(callID, f.FilePath, contract.ChangeUpdate, diff, nil))
		case patchDelete:
			events = append(events, editEvent(callID, f.FilePath, contract.ChangeDelete, diff, nil))
		case patchMove:
			events = append(events, moveEvents(callID, f, diff)...)
		default:
			events = append(events, editEvent(callID, f.FilePath, contract.ChangeUnknown, diff, nil))
		}
	}
	return events
}

func moveEvents(callID string, f patchFile, diff *string) []contract.Event {
	if f.MovePath == "" {
		return []contract.Event{editEvent(callID, f.FilePath, contract.ChangeUnknown, diff, nil)}
	}
	return []contract.Event{
		editEvent(callID, f.FilePath, contract.ChangeDelete, nil, nil),
		editEvent(callID, f.MovePath, contract.ChangeAdd, diff, nil),
	}
}

func editEvent(callID, path string, change contract.EditChange, diff, after *string) contract.Event {
	var cut adapter.Truncation
	edit := &contract.Edit{
		CallID: cut.Text("callId", callID, contract.MaxLabelBytes),
		Path:   cut.Text("path", path, contract.MaxPathBytes),
		Change: change,
		Diff:   cut.Optional("diff", diff, contract.MaxDiffBytes),
		After:  cut.Optional("after", after, contract.MaxDiffBytes),
	}
	return contract.Event{Kind: contract.KindEdit, Edit: edit, Truncated: cut.Fields()}
}

// hunks is a createTwoFilesPatch diff without its file header ("Index:",
// "===", "---", "+++"), as the Claude adapter's diffs are: empty when the
// patch has a header and no hunk (no change), nil when there is no patch.
// A diff without that header is kept as written.
func hunks(diff *string) *string {
	if diff == nil || !strings.HasPrefix(*diff, headerStart) {
		return diff
	}
	body := ""
	if i := strings.Index(*diff, "\n"+hunkStart); i >= 0 {
		body = (*diff)[i+1:]
	}
	return &body
}
