package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

func TestMessagesAndReasoning(t *testing.T) {
	t.Parallel()
	events := normalize(t,
		`{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"**Plan**"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Done."}}`,
	)
	if m := events[0].Message; m.Role != contract.RoleAssistant || m.Type != contract.MessageReasoning || m.Text != "**Plan**" || m.MessageID != "item_0" {
		t.Errorf("reasoning = %+v", m)
	}
	if m := events[1].Message; m.Type != contract.MessageText || m.Text != "Done." || m.MessageID != "item_1" {
		t.Errorf("agent message = %+v", m)
	}
}

func TestRepeatedMessageIsRenderedOnce(t *testing.T) {
	t.Parallel()
	line := `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Done."}}`
	events := normalize(t,
		`{"type":"item.started","item":{"id":"item_1","type":"agent_message","text":"Done."}}`,
		line,
		line,
		`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Done, really."}}`,
	)
	messages := ofKind(events, contract.KindMessage)
	if len(messages) != 2 || messages[0].Source.Line != 1 || messages[1].Message.Text != "Done, really." {
		t.Errorf("messages = %s, want the first and the changed one", dump(messages))
	}
}

func TestMessageIsBounded(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("é", contract.MaxTextBytes)
	events := normalize(t, `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"`+text+`"}}`)
	ev := only(t, events, contract.KindMessage)
	if len(ev.Message.Text) > contract.MaxTextBytes || len(ev.Truncated) != 1 || ev.Truncated[0] != "text" {
		t.Errorf("message of %d bytes, truncated %v", len(ev.Message.Text), ev.Truncated)
	}
}

func TestErrorItems(t *testing.T) {
	t.Parallel()
	line := `{"type":"item.completed","item":{"id":"item_0","type":"error","message":"model rerouted: a -> b"}}`
	events := normalize(t, line, line)
	if info := only(t, events, contract.KindError).Error; info.Message != "model rerouted: a -> b" || info.Retry != nil {
		t.Errorf("error item = %+v", info)
	}
}

func TestWebSearchKeepsTheItemID(t *testing.T) {
	t.Parallel()
	events := normalize(t, `{"type":"item.completed","item":{"id":"item_4","type":"web_search","id":"ws_example","query":"q","action":{"type":"search","query":"q"}}}`)
	call := only(t, events, contract.KindToolCall).ToolCall
	if call.CallID != "item_4" || call.Name != itemWebSearch {
		t.Errorf("call = %+v, want item_4", call)
	}
	if string(call.Input) != `{"action":{"type":"search","query":"q"},"query":"q"}` {
		t.Errorf("input = %s", call.Input)
	}
	result := only(t, events, contract.KindToolResult).ToolResult
	if result.CallID != "item_4" || result.Status != contract.ResultUnknown || result.Output != nil {
		t.Errorf("result = %+v, want unknown with no output", result)
	}
}

func TestFirstID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{`{"id":"a","x":{"id":"inner"},"id":"b"}`, "a", true},
		{`{"x":{"id":"inner"},"id":"b"}`, "b", true},
		{`{"x":1}`, "", false},
		{`{"id":7}`, "", false},
		{`["id","a"]`, "", false},
		{`{"id"`, "", false},
	}
	for _, c := range cases {
		got, ok := firstID(json.RawMessage(c.raw))
		if got != c.want || ok != c.ok {
			t.Errorf("firstID(%s) = %q, %v, want %q, %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}
