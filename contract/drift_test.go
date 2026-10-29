package contract

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// schemaDefs returns the schema's $defs, decoded.
func schemaDefs(t *testing.T) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Defs
}

// objectFields returns an object schema's property names and required set.
// For Event it also collects the payload properties of its oneOf branches.
func objectFields(t *testing.T, def map[string]any) (props []string, required map[string]bool) {
	t.Helper()
	required = map[string]bool{}
	for name := range asMap(def["properties"]) {
		props = append(props, name)
	}
	for _, name := range asSlice(def["required"]) {
		required[name.(string)] = true
	}
	for _, branch := range asSlice(def["oneOf"]) {
		for name := range asMap(asMap(branch)["properties"]) {
			if !slices.Contains(props, name) {
				props = append(props, name)
			}
		}
	}
	slices.Sort(props)
	return props, required
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func asSlice(v any) []any        { s, _ := v.([]any); return s }

// goFields returns a struct's JSON field names and which are omitempty.
func goFields(typ reflect.Type) (names []string, optional map[string]bool) {
	optional = map[string]bool{}
	for field := range typ.Fields() {
		name, opts, _ := strings.Cut(field.Tag.Get("json"), ",")
		names = append(names, name)
		optional[name] = strings.Contains(opts, "omitempty")
	}
	slices.Sort(names)
	return names, optional
}

// TestGoTypesMatchSchemaObjects checks that each Go type has exactly the
// schema object's properties, and that a property is optional in Go
// (omitempty) exactly when the schema does not require it.
func TestGoTypesMatchSchemaObjects(t *testing.T) {
	t.Parallel()
	defs := schemaDefs(t)
	types := map[string]any{
		"RunKey": RunKey{}, "AttemptKey": AttemptKey{}, "Run": Run{}, "Runner": Runner{},
		"ContainerState": ContainerState{}, "Coverage": Coverage{}, "Attempt": Attempt{},
		"Stream": Stream{}, "Limits": Limits{}, "Source": Source{}, "Message": Message{},
		"ToolCall": ToolCall{}, "ToolResult": ToolResult{}, "Edit": Edit{}, "Usage": Usage{},
		"Retry": Retry{}, "ErrorInfo": ErrorInfo{}, "Output": Output{}, "Lifecycle": Lifecycle{},
		"Raw": Raw{}, "Gap": Gap{}, "Event": Event{}, "Snapshot": Snapshot{},
		"AttemptUpdate": AttemptUpdate{}, "RunUpdate": RunUpdate{}, "Reset": Reset{}, "End": End{}, "Live": Live{},
		"ErrorResponse": ErrorResponse{}, "ContractInfo": ContractInfo{}, "SourceLine": SourceLine{},
		"RunList": RunList{},
	}
	for name, value := range types {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			def, ok := defs[name]
			if !ok {
				t.Fatalf("no $defs/%s", name)
			}
			checkObject(t, def, reflect.TypeOf(value))
		})
	}
	t.Run("ErrorBody", func(t *testing.T) {
		t.Parallel()
		body := asMap(asMap(defs["ErrorResponse"]["properties"])["error"])
		checkObject(t, body, reflect.TypeFor[ErrorBody]())
	})
}

func checkObject(t *testing.T, def map[string]any, typ reflect.Type) {
	t.Helper()
	props, required := objectFields(t, def)
	names, optional := goFields(typ)
	if !slices.Equal(props, names) {
		t.Fatalf("schema properties %v, Go fields %v", props, names)
	}
	for _, name := range names {
		if required[name] == optional[name] {
			t.Errorf("%s: schema required=%v, Go omitempty=%v", name, required[name], optional[name])
		}
	}
}

func enumOf(t *testing.T, schema map[string]any) []string {
	t.Helper()
	var values []string
	for _, v := range asSlice(schema["enum"]) {
		values = append(values, v.(string))
	}
	if len(values) == 0 {
		t.Fatal("no enum")
	}
	return values
}

func property(defs map[string]map[string]any, def, prop string) map[string]any {
	return asMap(asMap(defs[def]["properties"])[prop])
}

// enumCase pairs a schema enum with the Go constants that mirror it, in the
// schema's order.
type enumCase struct {
	name   string
	schema map[string]any
	goList []string
}

func runEnumCases(defs map[string]map[string]any) []enumCase {
	return []enumCase{
		{"run phases", property(defs, "Run", "phase"), toStrings([]RunPhase{
			PhasePending, PhaseWaiting, PhaseRunning, PhaseSucceeded, PhaseFailed, PhaseUnknown})},
		{"runner kinds", defs["RunnerKind"], toStrings([]RunnerKind{
			RunnerClaudeCode, RunnerCodex, RunnerOpenCode, RunnerUnknown})},
		{"container states", property(defs, "ContainerState", "state"), toStrings([]ContainerStateKind{
			ContainerWaiting, ContainerRunning, ContainerTerminated, ContainerUnknown})},
		{"coverage states", property(defs, "Coverage", "state"), toStrings([]CoverageState{
			CoveragePending, CoverageFollowing, CoverageEnded, CoverageUnavailable, CoverageExpired})},
	}
}

func eventEnumCases(defs map[string]map[string]any) []enumCase {
	return []enumCase{
		{"event kinds", property(defs, "Event", "kind"), toStrings(EventKinds)},
		{"message roles", property(defs, "Message", "role"), toStrings([]MessageRole{
			RoleUser, RoleAssistant})},
		{"message types", property(defs, "Message", "type"), toStrings([]MessageType{
			MessageText, MessageReasoning})},
		{"tool result statuses", property(defs, "ToolResult", "status"), toStrings([]ToolResultStatus{
			ResultOK, ResultError, ResultDeclined, ResultUnknown})},
		{"edit changes", property(defs, "Edit", "change"), toStrings([]EditChange{
			ChangeAdd, ChangeUpdate, ChangeDelete, ChangeUnknown})},
		{"usage scopes", property(defs, "Usage", "scope"), toStrings([]UsageScope{
			ScopeMessage, ScopeStep, ScopeTurn, ScopeSession})},
		{"lifecycle phases", property(defs, "Lifecycle", "phase"), toStrings([]LifecyclePhase{
			LifecycleSetupStarted, LifecycleSetupCompleted, LifecycleSetupFailed,
			LifecycleSessionStarted, LifecycleStepStarted, LifecycleStepCompleted,
			LifecycleTurnStarted, LifecycleTurnCompleted,
			LifecycleTurnFailed, LifecycleCompacted, LifecycleThinking, LifecycleSessionCompleted,
			LifecycleRateLimitStatus, LifecycleLimitHit, LifecycleKelosOutputs})},
		{"raw reasons", property(defs, "Raw", "reason"), toStrings([]RawReason{
			RawText, RawMalformedJSON, RawUnknownType, RawUnsupported, RawOversized})},
		{"gap reasons", property(defs, "Gap", "reason"), toStrings([]GapReason{
			GapSourceTruncated, GapReadInterrupted, GapInputLimit, GapAttemptExpired})},
	}
}

func apiEnumCases(defs map[string]map[string]any) []enumCase {
	return []enumCase{
		{"reset reasons", property(defs, "Reset", "reason"), toStrings([]ResetReason{
			ResetEpochChanged, ResetReplayExpired, ResetClientTooSlow, ResetBadCursor})},
		{"end reasons", property(defs, "End", "reason"), toStrings([]EndReason{
			EndRunFinished, EndRunDeleted})},
		{"error codes", asMap(asMap(property(defs, "ErrorResponse", "error")["properties"])["code"]),
			toStrings([]ErrorCode{ErrorInvalidRunKey, ErrorRunNotFound, ErrorNotImplemented, ErrorInternal, ErrorNotFound, ErrorMethodNotAllowed, ErrorUnavailable, ErrorInvalidEventID, ErrorSourceNotFound})},
	}
}

func TestGoConstantsMatchSchemaEnums(t *testing.T) {
	t.Parallel()
	defs := schemaDefs(t)
	cases := slices.Concat(runEnumCases(defs), eventEnumCases(defs), apiEnumCases(defs))
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := enumOf(t, c.schema); !slices.Equal(got, c.goList) {
				t.Errorf("schema enum %v, Go constants %v", got, c.goList)
			}
		})
	}
	if got := defs["SchemaVersion"]["const"]; got != SchemaVersion {
		t.Errorf("schema version %v, Go %q", got, SchemaVersion)
	}
}

func toStrings[S ~string](values []S) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

func TestSchemaBoundsMatchLimits(t *testing.T) {
	t.Parallel()
	defs := schemaDefs(t)
	maxLength := map[string]int{
		"Label": MaxLabelBytes, "Text": MaxTextBytes, "ToolInputText": MaxToolInputBytes,
		"ToolOutputText": MaxToolOutputBytes, "DiffText": MaxDiffBytes, "RawText": MaxRawBytes,
		"Path": MaxPathBytes, "SourceText": MaxSourceBytes,
	}
	for def, limit := range maxLength {
		if got := defs[def]["maxLength"]; got != float64(limit) {
			t.Errorf("$defs/%s maxLength %v, Go limit %d", def, got, limit)
		}
	}
	maxItems := []struct {
		schema map[string]any
		limit  int
	}{
		{property(defs, "Snapshot", "attempts"), MaxAttemptsPerRun},
		{property(defs, "Snapshot", "events"), MaxEventsPerRun},
		{property(defs, "Lifecycle", "outputs"), MaxOutputs},
		{property(defs, "Event", "truncated"), MaxTruncatedFields},
		{property(defs, "RunList", "runs"), MaxListedRuns},
	}
	for _, c := range maxItems {
		if got := c.schema["maxItems"]; got != float64(c.limit) {
			t.Errorf("maxItems %v, Go limit %d", got, c.limit)
		}
	}
}
