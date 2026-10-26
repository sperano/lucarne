package stream

import (
	"context"
	"encoding/json"

	"github.com/sperano/lucarne/contract"
	"github.com/sperano/lucarne/internal/adapter"
	"github.com/sperano/lucarne/internal/adapter/claude"
	"github.com/sperano/lucarne/internal/adapter/codex"
	"github.com/sperano/lucarne/internal/adapter/opencode"
	"github.com/sperano/lucarne/internal/kube"
)

// Source is where runs are read from: *kube.Source, or a test's.
type Source interface {
	Run(ctx context.Context, key contract.RunKey) (kube.Run, error)
	Refresh(ctx context.Context, run kube.Run) (kube.Run, error)
	Attempts(ctx context.Context, run kube.Run) ([]contract.Attempt, error)
	ReadLog(ctx context.Context, run kube.Run, key contract.AttemptKey, opts kube.ReadOptions, sink kube.LogSink) (kube.ReadResult, error)
}

var _ Source = (*kube.Source)(nil)

// newRunner returns the parser of one log of a runner of kind. A runner
// without adapter has every line kept raw.
func newRunner(kind contract.RunnerKind) adapter.Runner {
	switch kind {
	case contract.RunnerClaudeCode:
		return claude.New()
	case contract.RunnerCodex:
		return codex.New()
	case contract.RunnerOpenCode:
		return opencode.New()
	default:
		return rawRunner{}
	}
}

// rawRunner renders nothing: each line is raw, unknown_type when it is a
// JSON object, as no adapter knows its type.
type rawRunner struct{}

func (rawRunner) Line(text []byte, out *adapter.Emitter) {
	var object map[string]json.RawMessage
	if adapter.DecodeObject(text, out, &object) {
		out.RawLine(contract.RawUnknownType)
	}
}
