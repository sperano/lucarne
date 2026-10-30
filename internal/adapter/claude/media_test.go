package claude

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sperano/lucarne/contract"
)

// pngData is base64 of 12 bytes.
const (
	pngData  = "iVBORw0KGgoAAAAN"
	pngBytes = 12
)

// mediaResult is a user line whose only tool result has content blocks.
func mediaResult(blocks ...string) string {
	return `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[` +
		strings.Join(blocks, ",") + `]}]}}`
}

func size(n int64) *int64 { return &n }

func TestToolResultMedia(t *testing.T) {
	t.Parallel()
	events := normalize(t, mediaResult(
		`{"type":"text","text":"Read 1 page."}`,
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+pngData+`"}}`,
		`{"type":"document","title":"Design notes","source":{"type":"url","url":"https://docs.example.com/notes.pdf"}}`,
		`{"type":"document","source":{"type":"text","media_type":"text/plain","data":"plain"}}`,
		`{"type":"image","source":{"type":"file","file_id":"file_01Example"}}`,
	))
	result := only(t, events, contract.KindToolResult).ToolResult
	if result.Output == nil || *result.Output != "Read 1 page." {
		t.Errorf("output = %v, want the text block", result.Output)
	}
	want := []contract.Media{
		{Type: contract.MediaImage, MediaType: "image/png", ByteSize: size(pngBytes)},
		{Type: contract.MediaDocument, Title: "Design notes"},
		{Type: contract.MediaDocument, MediaType: "text/plain"},
		{Type: contract.MediaImage},
	}
	if !reflect.DeepEqual(result.Media, want) {
		t.Errorf("media = %s, want %+v", dump(events), want)
	}
	if raw := ofKind(events, contract.KindRaw); len(raw) != 0 {
		t.Errorf("raw events %s, want none", dump(raw))
	}
}

func TestMediaOnlyResultHasNoOutput(t *testing.T) {
	t.Parallel()
	events := normalize(t, mediaResult(
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+pngData+`"}}`,
	))
	result := only(t, events, contract.KindToolResult).ToolResult
	if result.Output != nil || len(result.Media) != 1 {
		t.Errorf("result = %s, want no output and one media entry", dump(events))
	}
}

func TestMediaBlocksItCannotReadStayRaw(t *testing.T) {
	t.Parallel()
	unread := []string{
		`{"type":"image"}`,
		`{"type":"image","source":{}}`,
		`{"type":"image","source":{"type":"text","data":"not an image source"}}`,
		`{"type":"document","source":{"type":"bogus"}}`,
		`{"type":"document","source":"not an object"}`,
	}
	events := normalize(t, mediaResult(unread...))
	if result := only(t, events, contract.KindToolResult).ToolResult; result.Media != nil {
		t.Errorf("media = %+v, want none", result.Media)
	}
	var texts []string
	for _, ev := range ofKind(events, contract.KindRaw) {
		texts = append(texts, ev.Raw.Text)
	}
	if !slices.Equal(texts, unread) {
		t.Errorf("raw blocks %q, want %q", texts, unread)
	}
}

func TestMediaByteSizeIsOnlyWhatTheSourceGives(t *testing.T) {
	t.Parallel()
	events := normalize(t, mediaResult(
		`{"type":"image","source":{"type":"base64","media_type":"image/png"}}`,
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"not base64!"}}`,
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":""}}`,
		`{"type":"image","title":"ignored","source":{"type":"url","url":"https://img.example.com/a.png"}}`,
	))
	want := []contract.Media{
		{Type: contract.MediaImage, MediaType: "image/png"},
		{Type: contract.MediaImage, MediaType: "image/png"},
		{Type: contract.MediaImage, MediaType: "image/png", ByteSize: size(0)},
		{Type: contract.MediaImage},
	}
	if got := only(t, events, contract.KindToolResult).ToolResult.Media; !reflect.DeepEqual(got, want) {
		t.Errorf("media = %s, want %+v", dump(events), want)
	}
}

func TestMediaIsBounded(t *testing.T) {
	t.Parallel()
	blocks := make([]string, contract.MaxMedia+1)
	for i := range blocks {
		blocks[i] = fmt.Sprintf(`{"type":"document","title":"doc %d","source":{"type":"file","file_id":"f"}}`, i)
	}
	long := strings.Repeat("é", contract.MaxLabelBytes)
	events := normalize(t,
		mediaResult(blocks...),
		mediaResult(`{"type":"document","title":"`+long+`","source":{"type":"file","file_id":"f"}}`),
	)
	results := ofKind(events, contract.KindToolResult)
	if len(results) != 2 {
		t.Fatalf("got %s, want two results", dump(events))
	}
	many := results[0]
	if len(many.ToolResult.Media) != contract.MaxMedia || !slices.Equal(many.Truncated, []string{mediaField}) {
		t.Errorf("%d media, truncated %v, want %d and media cut", len(many.ToolResult.Media), many.Truncated, contract.MaxMedia)
	}
	titled := results[1]
	if got := titled.ToolResult.Media[0].Title; len(got) > contract.MaxLabelBytes || !slices.Equal(titled.Truncated, []string{mediaField}) {
		t.Errorf("title of %d bytes, truncated %v, want it cut to %d", len(got), titled.Truncated, contract.MaxLabelBytes)
	}
}
