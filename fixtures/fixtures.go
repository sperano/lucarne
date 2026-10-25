// Package fixtures holds sanitized, provenance-labelled runner output for
// adapter tests. manifest.json says where each file comes from and what it
// covers; README.md explains the layout and how to add a capture.
package fixtures

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

// ManifestFile is the manifest's path inside Files.
const ManifestFile = "manifest.json"

// Files holds the manifest and every fixture, as <runner>/<version>/<name>.log.
//
//go:embed manifest.json */*/*.log
var Files embed.FS

// Provenance labels.
const (
	ProvenanceSynthetic = "synthetic"
	ProvenanceCaptured  = "captured"
)

// Manifest describes every fixture.
type Manifest struct {
	ManifestVersion int                          `json:"manifestVersion"`
	Deployed        Deployed                     `json:"deployed"`
	CaptureGap      string                       `json:"captureGap"`
	Basis           map[string][]string          `json:"basis"`
	NotObservable   map[string]map[string]string `json:"notObservable"`
	Fixtures        []Fixture                    `json:"fixtures"`
}

// Deployed records the runner versions deployed when the fixtures were
// written, and where that was read.
type Deployed struct {
	RecordedOn string            `json:"recordedOn"`
	Kelos      string            `json:"kelos"`
	Runners    map[string]string `json:"runners"`
	Source     string            `json:"source"`
}

// Fixture is one file's provenance and content summary.
type Fixture struct {
	File       string     `json:"file"`
	Runner     string     `json:"runner"`
	Version    string     `json:"version"`
	Provenance string     `json:"provenance"`
	CapturedOn string     `json:"capturedOn,omitempty"`
	Scenarios  []string   `json:"scenarios"`
	Lines      LineCounts `json:"lines"`
	Notes      string     `json:"notes"`
}

// LineCounts classifies a fixture's lines: empty, a JSON object, or
// anything else.
type LineCounts struct {
	Blank int `json:"blank"`
	JSON  int `json:"json"`
	Text  int `json:"text"`
}

// LoadManifest decodes the manifest, rejecting unknown fields.
func LoadManifest() (Manifest, error) {
	data, err := fs.ReadFile(Files, ManifestFile)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := decodeStrict(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("decode %s: %w", ManifestFile, err)
	}
	return m, nil
}

// Read returns a fixture's content.
func Read(file string) ([]byte, error) { return fs.ReadFile(Files, file) }

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
