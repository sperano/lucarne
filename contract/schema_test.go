package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/sperano/lucarne/internal/privacy"
)

const (
	validDir   = "v1/examples"
	invalidDir = "v1/invalid"
)

// example is one example document and the definition it targets, named by
// its directory.
type example struct {
	def, path string
	data      []byte
}

func loadExamples(t *testing.T, root string) []example {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no examples under %s", root)
	}
	examples := make([]example, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		def := filepath.Base(filepath.Dir(p))
		if !slices.Contains(Documents, def) {
			t.Fatalf("%s: directory %q is not a document definition", p, def)
		}
		examples = append(examples, example{def: def, path: p, data: data})
	}
	return examples
}

func TestValidExamplesValidate(t *testing.T) {
	for _, ex := range loadExamples(t, validDir) {
		t.Run(ex.path, func(t *testing.T) {
			if err := Validate(ex.def, ex.data); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestInvalidExamplesAreRejected(t *testing.T) {
	for _, ex := range loadExamples(t, invalidDir) {
		t.Run(ex.path, func(t *testing.T) {
			err := Validate(ex.def, ex.data)
			var verr *jsonschema.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("got %v, want a wrapped *jsonschema.ValidationError", err)
			}
		})
	}
}

func TestEveryDocumentHasAValidExample(t *testing.T) {
	seen := map[string]bool{}
	for _, ex := range loadExamples(t, validDir) {
		seen[ex.def] = true
	}
	for _, def := range Documents {
		if !seen[def] {
			t.Errorf("no valid example for %s", def)
		}
	}
}

func TestEveryEventKindHasAnExample(t *testing.T) {
	seen := map[EventKind]bool{}
	for _, ex := range loadExamples(t, validDir) {
		for _, e := range eventsOf(t, ex) {
			seen[e.Kind] = true
		}
	}
	for _, kind := range EventKinds {
		if !seen[kind] {
			t.Errorf("no example event of kind %q", kind)
		}
	}
}

func eventsOf(t *testing.T, ex example) []Event {
	t.Helper()
	switch ex.def {
	case DefEvent:
		var e Event
		mustDecode(t, ex.data, &e)
		return []Event{e}
	case DefSnapshot:
		var s Snapshot
		mustDecode(t, ex.data, &s)
		return s.Events
	}
	return nil
}

// TestExamplesRoundTripThroughGoTypes proves the Go types hold every field
// of every example: decoding rejects unknown fields and encoding gives back
// the same document.
func TestExamplesRoundTripThroughGoTypes(t *testing.T) {
	for _, ex := range loadExamples(t, validDir) {
		t.Run(ex.path, func(t *testing.T) {
			target := newDocument(t, ex.def)
			mustDecode(t, ex.data, target)
			encoded, err := json.Marshal(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(ex.def, encoded); err != nil {
				t.Errorf("re-encoded document is invalid: %v", err)
			}
			var want, got any
			mustDecode(t, ex.data, &want)
			mustDecode(t, encoded, &got)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("round trip changed the document:\nwant %s\ngot  %s", ex.data, encoded)
			}
		})
	}
}

func newDocument(t *testing.T, def string) any {
	t.Helper()
	documents := map[string]any{
		DefSnapshot:      &Snapshot{},
		DefEvent:         &Event{},
		DefAttemptUpdate: &AttemptUpdate{},
		DefRunUpdate:     &RunUpdate{},
		DefReset:         &Reset{},
		DefEnd:           &End{},
		DefErrorResponse: &ErrorResponse{},
		DefContractInfo:  &ContractInfo{},
	}
	doc, ok := documents[def]
	if !ok {
		t.Fatalf("no Go type for %s", def)
	}
	return doc
}

func mustDecode(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestExampleLimitsAreTheDefaults(t *testing.T) {
	for _, ex := range loadExamples(t, validDir) {
		var limited struct {
			Limits *Limits `json:"limits"`
		}
		if err := json.Unmarshal(ex.data, &limited); err != nil {
			t.Fatal(err)
		}
		if limited.Limits != nil && *limited.Limits != DefaultLimits() {
			t.Errorf("%s: limits %+v, want %+v", ex.path, *limited.Limits, DefaultLimits())
		}
	}
}

func TestExamplesHoldNoPrivateData(t *testing.T) {
	for _, root := range []string{validDir, invalidDir} {
		for _, ex := range loadExamples(t, root) {
			for _, finding := range privacy.Scan(string(ex.data)) {
				t.Errorf("%s: %v", ex.path, finding)
			}
		}
	}
}

func TestValidateRejectsUnknownDefinition(t *testing.T) {
	if err := Validate("Nope", []byte(`{}`)); err == nil {
		t.Error("an unknown definition validated")
	}
}
