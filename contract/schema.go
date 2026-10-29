// Package contract is Lucarne's versioned transcript and API contract: the
// Go types of the wire format, the identities of runs, attempts and events,
// the limits, and the JSON Schema both the server and the UI validate
// against. README.md in this directory is the specification.
package contract

import (
	"bytes"
	_ "embed"
	"fmt"

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
	DefLive          = "Live"
	DefErrorResponse = "ErrorResponse"
	DefContractInfo  = "ContractInfo"
	DefSourceLine    = "SourceLine"
	DefRunList       = "RunList"
)

// Documents lists the definitions a whole wire document is validated
// against.
var Documents = []string{
	DefSnapshot, DefEvent, DefAttemptUpdate, DefRunUpdate,
	DefReset, DefEnd, DefLive, DefErrorResponse, DefContractInfo, DefSourceLine,
	DefRunList,
}

// compiled maps each of Documents to its compiled schema. It is compiled
// once, at init: the schema is embedded, so a failure is a programmer error.
var compiled = mustCompileAll()

// Validate checks a JSON document against one definition of the schema.
func Validate(def string, document []byte) error {
	schema, ok := compiled[def]
	if !ok {
		return fmt.Errorf("unknown definition %q", def)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return fmt.Errorf("decode %s: %w", def, err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("validate %s: %w", def, err)
	}
	return nil
}

// mustCompileAll compiles every definition of Documents from the embedded
// schema. It panics when the schema does not decode or compile.
func mustCompileAll() map[string]*jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("contract: decode schema: %v", err))
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(SchemaID, doc); err != nil {
		panic(fmt.Sprintf("contract: add schema: %v", err))
	}
	schemas := make(map[string]*jsonschema.Schema, len(Documents))
	for _, def := range Documents {
		schema, err := compiler.Compile(SchemaID + "#/$defs/" + def)
		if err != nil {
			panic(fmt.Sprintf("contract: compile %s: %v", def, err))
		}
		schemas[def] = schema
	}
	return schemas
}
