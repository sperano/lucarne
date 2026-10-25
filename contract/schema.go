// Package contract is Lucarne's versioned transcript and API contract: the
// Go types of the wire format, the identities of runs, attempts and events,
// the limits, and the JSON Schema both the server and the UI validate
// against. README.md in this directory is the specification.
package contract

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaID is the schema's $id, which its definitions are resolved against.
const SchemaID = "https://github.com/sperano/lucarne/contract/v1/transcript.schema.json"

// SchemaJSON is the schema document.
//
//go:embed v1/transcript.schema.json
var SchemaJSON []byte

// Definitions a document can be validated against.
const (
	DefSnapshot      = "Snapshot"
	DefEvent         = "Event"
	DefAttemptUpdate = "AttemptUpdate"
	DefRunUpdate     = "RunUpdate"
	DefReset         = "Reset"
	DefEnd           = "End"
	DefErrorResponse = "ErrorResponse"
	DefContractInfo  = "ContractInfo"
)

// Documents lists the definitions a whole wire document is validated
// against.
var Documents = []string{
	DefSnapshot, DefEvent, DefAttemptUpdate, DefRunUpdate,
	DefReset, DefEnd, DefErrorResponse, DefContractInfo,
}

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

// Validate checks a JSON document against one definition of the schema.
func Validate(def string, document []byte) error {
	compileOnce.Do(compileAll)
	if compileErr != nil {
		return compileErr
	}
	schema, ok := compiled[def]
	if !ok {
		return fmt.Errorf("unknown definition %q", def)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return fmt.Errorf("decode %s: %w", def, err)
	}
	if err := schema.Validate(value); err != nil {
		// %#v prints the library's detail: each failing keyword and path.
		return fmt.Errorf("%#v", err)
	}
	return nil
}

func compileAll() {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
	if err != nil {
		compileErr = fmt.Errorf("decode schema: %w", err)
		return
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(SchemaID, doc); err != nil {
		compileErr = err
		return
	}
	compiled = make(map[string]*jsonschema.Schema, len(Documents))
	for _, def := range Documents {
		schema, err := compiler.Compile(SchemaID + "#/$defs/" + def)
		if err != nil {
			compileErr = fmt.Errorf("compile %s: %w", def, err)
			return
		}
		compiled[def] = schema
	}
}
