package opencode

import (
	"testing"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter/adaptertest"
)

func editsOf(t *testing.T, tool, state string) []contract.Edit {
	t.Helper()
	var found []contract.Edit
	for _, ev := range adaptertest.OfKind(normalize(t, toolLine(tool, state)), contract.KindEdit) {
		if ev.Edit.CallID != "call_a" {
			t.Errorf("edit call ID = %q", ev.Edit.CallID)
		}
		found = append(found, *ev.Edit)
	}
	return found
}

func TestHunks(t *testing.T) {
	t.Parallel()
	header := "Index: /a\n===================================================================\n--- /a\n+++ /a\n"
	cases := []struct {
		name, diff, want string
	}{
		{"header and hunks", header + "@@ -1 +1 @@\n-a\n+b\n", "@@ -1 +1 @@\n-a\n+b\n"},
		{"header only", header, ""},
		{"no header", "@@ -1 +1 @@\n-a\n+b\n", "@@ -1 +1 @@\n-a\n+b\n"},
		{"not a diff", "example", "example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := hunks(&c.diff); got == nil || *got != c.want {
				t.Errorf("hunks = %v, want %q", got, c.want)
			}
		})
	}
	if hunks(nil) != nil {
		t.Error("no diff gave a diff")
	}
}

func TestEditChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		state string
		want  contract.EditChange
		path  string
	}{
		{"update", `{"status":"completed","input":{"filePath":"a.go","oldString":"x","newString":"y"},"output":"","metadata":{"diff":"@@ -1 +1 @@\n"}}`,
			contract.ChangeUpdate, "a.go"},
		{"create", `{"status":"completed","input":{"filePath":"a.go","oldString":"","newString":"y"},"output":"","metadata":{"filediff":{"file":"/w/a.go"}}}`,
			contract.ChangeAdd, "/w/a.go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			edits := editsOf(t, "edit", c.state)
			if len(edits) != 1 || edits[0].Change != c.want || edits[0].Path != c.path {
				t.Errorf("edits = %+v", edits)
			}
		})
	}
	if edits := editsOf(t, "edit", `{"status":"completed","input":{"filePath":"a.go"},"output":"","metadata":{}}`); edits != nil {
		t.Errorf("an edit without oldString gave %+v", edits)
	}
	if edits := editsOf(t, "edit", `{"status":"error","input":{"filePath":"a.go","oldString":"x"},"error":"no","metadata":{"diff":""}}`); edits != nil {
		t.Errorf("a failed edit gave %+v", edits)
	}
}

func TestWriteChange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, metadata string
		want           contract.EditChange
	}{
		{"new", `{"filepath":"/w/a","exists":false}`, contract.ChangeAdd},
		{"existing", `{"filepath":"/w/a","exists":true}`, contract.ChangeUpdate},
		{"unsaid", `{"filepath":"/w/a"}`, contract.ChangeUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			edits := editsOf(t, "write", `{"status":"completed","input":{"filePath":"a","content":"x\n"},"output":"","metadata":`+c.metadata+`}`)
			if len(edits) != 1 || edits[0].Change != c.want || edits[0].Path != "/w/a" || edits[0].After == nil ||
				*edits[0].After != "x\n" || edits[0].Diff != nil || edits[0].Before != nil {
				t.Errorf("edits = %+v", edits)
			}
		})
	}
}

func TestApplyPatchChanges(t *testing.T) {
	t.Parallel()
	state := `{"status":"completed","input":{},"output":"","metadata":{"files":[` +
		`{"filePath":"/w/m","type":"move","movePath":"/w/n","patch":"@@ -1 +1 @@\n"},` +
		`{"filePath":"/w/o","type":"move"},` +
		`{"filePath":"/w/f","type":"example-future"},` +
		`{"type":"add"}]}}`
	want := []struct {
		path   string
		change contract.EditChange
		diff   bool
	}{
		{"/w/m", contract.ChangeDelete, false},
		{"/w/n", contract.ChangeAdd, true},
		{"/w/o", contract.ChangeUnknown, false},
		{"/w/f", contract.ChangeUnknown, false},
	}
	edits := editsOf(t, "apply_patch", state)
	if len(edits) != len(want) {
		t.Fatalf("edits = %+v", edits)
	}
	for i, w := range want {
		if e := edits[i]; e.Path != w.path || e.Change != w.change || (e.Diff != nil) != w.diff {
			t.Errorf("edit %d = %+v, want %+v", i, e, w)
		}
	}
}

func TestEditsOnlyFromFileTools(t *testing.T) {
	t.Parallel()
	state := `{"status":"completed","input":{"filePath":"a","oldString":""},"output":"","metadata":{"files":[{"filePath":"a","type":"add"}],"filepath":"a"}}`
	if edits := editsOf(t, "bash", state); edits != nil {
		t.Errorf("bash gave %+v", edits)
	}
	if edits := editsOf(t, "write", `{"status":"completed","input":{},"output":"","metadata":{"filepath":7}}`); edits != nil {
		t.Errorf("metadata of another shape gave %+v", edits)
	}
}
