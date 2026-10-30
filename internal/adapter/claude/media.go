package claude

import (
	"encoding/base64"
	"io"
	"slices"
	"strings"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
)

// Source types of image and document blocks.
const (
	sourceBase64  = "base64"
	sourceURL     = "url"
	sourceFile    = "file"
	sourceText    = "text"
	sourceContent = "content"
)

// mediaField is the truncated field of a cut media entry.
const mediaField = "media"

// mediaKinds maps the media block types to their contract kind.
var mediaKinds = map[string]contract.MediaKind{
	blockImage:    contract.MediaImage,
	blockDocument: contract.MediaDocument,
}

// mediaSources are the source types the API defines for each media block.
var mediaSources = map[string][]string{
	blockImage:    {sourceBase64, sourceURL, sourceFile},
	blockDocument: {sourceBase64, sourceText, sourceContent, sourceURL, sourceFile},
}

// mediaSource is an image or document block's source. Data is the payload
// of a base64 or text source.
type mediaSource struct {
	Type      string  `json:"type"`
	MediaType string  `json:"media_type"`
	Data      *string `json:"data"`
}

// readableMedia is whether b is an image or document block with a source of
// a type the API defines for it.
func readableMedia(b block) bool {
	_, ok := mediaKinds[b.Type]
	return ok && b.Source != nil && slices.Contains(mediaSources[b.Type], b.Source.Type)
}

// blockMedia describes an image or document block of a message or tool
// result, never its bytes. It is false for another block, and for one
// without a source of a type the API defines for it.
func blockMedia(b block, cut *adapter.Truncation) (contract.Media, bool) {
	if !readableMedia(b) {
		return contract.Media{}, false
	}
	kind := mediaKinds[b.Type]
	m := contract.Media{
		Type:      kind,
		MediaType: cut.Text(mediaField, b.Source.MediaType, contract.MaxLabelBytes),
	}
	if b.Source.Type == sourceBase64 {
		m.ByteSize = decodedSize(b.Source.Data)
	}
	if kind == contract.MediaDocument && b.Title != nil {
		m.Title = cut.Text(mediaField, *b.Title, contract.MaxLabelBytes)
	}
	return m, true
}

// decodedSize is the size of base64 data once decoded, nil when there is no
// data or it is not standard base64.
func decodedSize(data *string) *int64 {
	if data == nil {
		return nil
	}
	n, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(*data)))
	if err != nil {
		return nil
	}
	return &n
}
