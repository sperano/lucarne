package claude

import (
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestMessageLinesShareTheirIDAndAreNotMerged(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"thinking","thinking":"plan"}],"stop_reason":null,"usage":{"output_tokens":2}}}`,
		`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}],"stop_reason":null,"usage":{"output_tokens":5}}}`,
		`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"last"}],"stop_reason":"end_turn","usage":{"output_tokens":9}}}`,
	)
	messages := ofKind(events, contract.KindMessage)
	want := []string{"plan", "first", "second", "last"}
	if len(messages) != len(want) {
		t.Fatalf("got %d messages, want %d: %s", len(messages), len(want), dump(events))
	}
	for i, m := range messages {
		if m.Message.Text != want[i] || m.Message.MessageID != "msg_1" {
			t.Errorf("message %d = %+v, want %q of msg_1", i, m.Message, want[i])
		}
	}
	if messages[0].Message.Type != contract.MessageReasoning {
		t.Errorf("thinking is %s, want reasoning", messages[0].Message.Type)
	}
	if messages[2].Source.Line != 2 || messages[2].Source.Part != 1 {
		t.Errorf("the second block of line 2 is at %+v, want part 1", messages[2].Source)
	}
	usage := only(t, events, contract.KindUsage)
	if usage.Source.Line != 3 || *usage.Usage.OutputTokens != 9 || usage.Usage.Scope != contract.ScopeMessage {
		t.Errorf("usage = %+v from line %d, want the final line's", usage.Usage, usage.Source.Line)
	}
}

func TestPartialUsageIsNotReported(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}],"stop_reason":null,"usage":{"input_tokens":3,"output_tokens":1}}}`,
	)
	if usage := ofKind(events, contract.KindUsage); len(usage) != 0 {
		t.Errorf("a line without stop_reason reported usage: %s", dump(usage))
	}
}

func TestToolCallInputAndParent(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("x", contract.MaxToolInputBytes)
	events := normalize(t,
		`{"type":"assistant","parent_tool_use_id":"toolu_task","message":{"id":"m","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{ "file_path" : "a.go" }}]}}`,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"toolu_2","name":"Write","input":{"content":"`+big+`"}}]}}`,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","id":"toolu_3","name":"Bash"}]}}`,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"tool_use","name":"Bash","input":{}}]}}`,
	)
	small := events[0].ToolCall
	if string(small.Input) != `{"file_path":"a.go"}` || small.ParentCallID != "toolu_task" {
		t.Errorf("call = %+v, want compact input and the subagent's parent", small)
	}
	cut := events[1]
	if cut.ToolCall.Input != nil || len(cut.ToolCall.InputText) != contract.MaxToolInputBytes || cut.Truncated[0] != "inputText" {
		t.Errorf("oversized input: input %d bytes, inputText %d bytes, truncated %v",
			len(cut.ToolCall.Input), len(cut.ToolCall.InputText), cut.Truncated)
	}
	if none := events[2].ToolCall; none.Input != nil || none.InputText != "" {
		t.Errorf("a call without input got %+v", none)
	}
	if events[3].Kind != contract.KindRaw || events[3].Raw.Reason != contract.RawUnsupported {
		t.Errorf("a tool_use without id is %s, want raw unsupported", events[3].Kind)
	}
}

func TestAPIErrorMessages(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","error":"rate_limit","message":{"id":"m","content":[{"type":"text","text":"Usage limit reached."}],"stop_reason":"stop_sequence"}}`,
		`{"type":"assistant","error":"authentication_failed","message":{"id":"m","content":[]}}`,
	)
	if len(ofKind(events, contract.KindMessage)) != 0 {
		t.Errorf("an API error was shown as an assistant message: %s", dump(events))
	}
	errs := ofKind(events, contract.KindError)
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %s", len(errs), dump(events))
	}
	if errs[0].Error.Message != "Usage limit reached." || errs[0].Error.Code != "rate_limit" {
		t.Errorf("error = %+v", errs[0].Error)
	}
	if errs[1].Error.Message != "authentication_failed" || errs[1].Error.Code != "authentication_failed" {
		t.Errorf("an error without text = %+v, want its code as the message", errs[1].Error)
	}
}

func TestUnrenderedBlocksStayRawInOrder(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"assistant","message":{"id":"m","content":[{"type":"text","text":"a"},{"type":"redacted_thinking","data":"opaque"},{"type":"example_block"},7,{"type":"text","text":"b"}]}}`,
	)
	wantReasons := []contract.RawReason{"", contract.RawUnsupported, contract.RawUnknownType, contract.RawUnsupported, ""}
	if len(events) != len(wantReasons) {
		t.Fatalf("got %d events, want %d: %s", len(events), len(wantReasons), dump(events))
	}
	for i, ev := range events {
		if ev.Source.Part != i {
			t.Errorf("event %d has part %d", i, ev.Source.Part)
		}
		if wantReasons[i] == "" {
			if ev.Kind != contract.KindMessage {
				t.Errorf("event %d is %s, want a message", i, ev.Kind)
			}
		} else if ev.Kind != contract.KindRaw || ev.Raw.Reason != wantReasons[i] {
			t.Errorf("event %d = %s %+v, want raw %s", i, ev.Kind, ev.Raw, wantReasons[i])
		}
	}
}

func TestTextIsBoundedAtARuneBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("é", contract.MaxTextBytes)
	events := normalize(t, `{"type":"assistant","message":{"id":"m","content":[{"type":"text","text":"`+text+`"}]}}`)
	m := only(t, events, contract.KindMessage)
	if len(m.Message.Text) != contract.MaxTextBytes || !strings.HasPrefix(text, m.Message.Text) {
		t.Errorf("text is %d bytes, want the first %d", len(m.Message.Text), contract.MaxTextBytes)
	}
	if len(m.Truncated) != 1 || m.Truncated[0] != "text" {
		t.Errorf("truncated = %v, want [text]", m.Truncated)
	}
}
