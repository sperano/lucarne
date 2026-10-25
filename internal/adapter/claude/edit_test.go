package claude

import (
	"testing"

	"github.com/sperano/lucarne/contract"
)

// toolUse is an assistant line calling tool as id.
func toolUse(id, tool string) string {
	return `{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"` + id + `","name":"` + tool + `","input":{}}]}}`
}

// toolResult is a user line with id's result and structured output.
func toolResult(id string, isError bool, structured string) string {
	errorField := "false"
	if isError {
		errorField = "true"
	}
	return `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"` + id +
		`","content":"done","is_error":` + errorField + `}]},"tool_use_result":` + structured + `}`
}

const (
	editOutput     = `{"filePath":"/workspace/repo/a.go","oldString":"a","newString":"b","structuredPatch":[{"oldStart":1,"oldLines":2,"newStart":1,"newLines":2,"lines":[" x","-a","+b"]},{"oldStart":9,"oldLines":1,"newStart":9,"newLines":0,"lines":["-z","\\ No newline at end of file"]}]}`
	createOutput   = `{"type":"create","filePath":"/workspace/repo/new.go","content":"package widget\n","structuredPatch":[]}`
	overOutput     = `{"type":"update","filePath":"/workspace/repo/a.go","content":"","structuredPatch":[{"oldStart":1,"oldLines":1,"newStart":0,"newLines":0,"lines":["-a"]}]}`
	readOutput     = `{"type":"text","file":{"filePath":"/workspace/repo/a.go","content":"a"}}`
	editDiff       = "@@ -1,2 +1,2 @@\n x\n-a\n+b\n@@ -9,1 +9,0 @@\n-z\n\\ No newline at end of file\n"
	emptiedDiff    = "@@ -1,1 +0,0 @@\n-a\n"
	newFileContent = "package widget\n"
)

func TestEdits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		lines  []string
		change contract.EditChange
		diff   *string
		after  *string
	}{
		{"edit", []string{toolUse("t", "Edit"), toolResult("t", false, editOutput)}, contract.ChangeUpdate, ptr(editDiff), nil},
		{"multi edit", []string{toolUse("t", "MultiEdit"), toolResult("t", false, editOutput)}, contract.ChangeUpdate, ptr(editDiff), nil},
		{"write a new file", []string{toolUse("t", "Write"), toolResult("t", false, createOutput)}, contract.ChangeAdd, nil, ptr(newFileContent)},
		{"write over a file", []string{toolUse("t", "Write"), toolResult("t", false, overOutput)}, contract.ChangeUpdate, ptr(emptiedDiff), ptr("")},
		{"edit of an unseen call", []string{toolResult("t", false, editOutput)}, contract.ChangeUpdate, ptr(editDiff), nil},
		{"write of an unseen call", []string{toolResult("t", false, createOutput)}, contract.ChangeAdd, nil, ptr(newFileContent)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			edit := only(t, normalize(t, c.lines...), contract.KindEdit).Edit
			if edit.CallID != "t" || edit.Change != c.change || !equal(edit.Diff, c.diff) || !equal(edit.After, c.after) {
				t.Errorf("edit = %+v (diff %v, after %v), want %s diff %v after %v",
					edit, deref(edit.Diff), deref(edit.After), c.change, deref(c.diff), deref(c.after))
			}
		})
	}
}

func TestNoEdit(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"failed edit":      {toolUse("t", "Edit"), toolResult("t", true, `"Error: String to replace not found"`)},
		"error with patch": {toolUse("t", "Edit"), toolResult("t", true, editOutput)},
		"read":             {toolUse("t", "Read"), toolResult("t", false, readOutput)},
		"unseen read":      {toolResult("t", false, readOutput)},
		"other tool":       {toolUse("t", "NotebookEdit"), toolResult("t", false, editOutput)},
		"no path":          {toolUse("t", "Edit"), toolResult("t", false, `{"structuredPatch":[]}`)},
		"two results": {toolUse("t", "Edit"), `{"type":"user","message":{"content":[` +
			`{"type":"tool_result","tool_use_id":"t","content":"done"},{"type":"tool_result","tool_use_id":"u","content":"done"}]},"tool_use_result":` + editOutput + `}`},
	}
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if edits := ofKind(normalize(t, lines...), contract.KindEdit); len(edits) != 0 {
				t.Errorf("got an edit: %s", dump(edits))
			}
		})
	}
}

func ptr(s string) *string { return &s }

func equal(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
