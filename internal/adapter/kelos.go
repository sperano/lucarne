package adapter

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/sperano/lucarne/contract"
)

// Kelos marker lines (kelos v0.60.0 */kelos_entrypoint.sh and
// internal/capture/capture.go, hollingsworth kelos_entrypoint.sh).
const (
	setupStartMarker   = "---KELOS_SETUP_COMMAND_START---"
	setupDoneMarker    = "---KELOS_SETUP_COMMAND_DONE---"
	setupFailedMarker  = "---KELOS_SETUP_COMMAND_FAILED---"
	outputsStartMarker = "---KELOS_OUTPUTS_START---"
	outputsEndMarker   = "---KELOS_OUTPUTS_END---"
	limitHitMarker     = "---KELOS_LIMIT_HIT---"
	// exitPrefix starts the argument of the setup failure marker.
	exitPrefix = "exit="
	// captureErrorKey starts kelos-capture's own errors on stderr, which
	// look like an output line and are not one.
	captureErrorKey = "kelos-capture"
)

// outputLine is a "key: value" line of the outputs block; the value may be
// empty.
var outputLine = regexp.MustCompile(`^([a-z][a-z0-9-]*):(?: (.*))?$`)

// outputsBlock is a Kelos outputs block read up to its end marker.
type outputsBlock struct {
	start   Line
	outputs []contract.Output
	cut     Truncation
}

// kelos adds the event of a Kelos marker line and reports whether the line
// was one, or a line of the outputs block being read. A line of the block
// that is not "key: value" (stderr written meanwhile) is not consumed.
func (n *Normalizer) kelos(out *Emitter) bool {
	text := string(out.line.Text)
	if n.outputs != nil {
		switch {
		case text == outputsEndMarker:
			out.events = append(out.events, n.flushOutputs()...)
			return true
		case n.outputs.add(text):
			return true
		}
	}
	switch {
	case text == setupStartMarker:
		out.Emit(lifecycle(contract.LifecycleSetupStarted))
	case text == setupDoneMarker:
		out.Emit(lifecycle(contract.LifecycleSetupCompleted))
	case text == outputsStartMarker:
		out.events = append(out.events, n.flushOutputs()...)
		n.outputs = &outputsBlock{start: Line{Number: out.line.Number, LogTime: out.line.LogTime}}
	default:
		if arg, ok := markerArgument(text, setupFailedMarker); ok {
			out.Emit(setupFailed(arg))
		} else if arg, ok := markerArgument(text, limitHitMarker); ok {
			out.Emit(withDetail(contract.LifecycleLimitHit, arg))
		} else {
			return false
		}
	}
	return true
}

// flushOutputs returns the outputs block being read as one event of its
// start line, and ends it.
func (n *Normalizer) flushOutputs() []contract.Event {
	block := n.outputs
	if block == nil {
		return nil
	}
	n.outputs = nil
	out := n.emitter(block.start)
	ev := lifecycle(contract.LifecycleKelosOutputs)
	ev.Lifecycle.Outputs = block.outputs
	ev.Truncated = block.cut.Fields()
	out.Emit(ev)
	return out.events
}

// add records a "key: value" line and reports whether text was one.
func (b *outputsBlock) add(text string) bool {
	m := outputLine.FindStringSubmatch(text)
	if m == nil || m[1] == captureErrorKey {
		return false
	}
	if len(b.outputs) == contract.MaxOutputs {
		b.cut.Mark("outputs")
		return true
	}
	key, keyCut := Cut(m[1], contract.MaxLabelBytes)
	value, valueCut := Cut(m[2], contract.MaxTextBytes)
	if keyCut || valueCut {
		b.cut.Mark("outputs")
	}
	b.outputs = append(b.outputs, contract.Output{Key: key, Value: value})
	return true
}

// markerArgument reports whether text is marker, alone or followed by a
// space and an argument, and returns the argument.
func markerArgument(text, marker string) (string, bool) {
	rest, ok := strings.CutPrefix(text, marker)
	if !ok || (rest != "" && rest[0] != ' ') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func setupFailed(arg string) contract.Event {
	if code, ok := strings.CutPrefix(arg, exitPrefix); ok {
		if exit, err := strconv.Atoi(code); err == nil {
			ev := lifecycle(contract.LifecycleSetupFailed)
			ev.Lifecycle.ExitCode = &exit
			return ev
		}
	}
	return withDetail(contract.LifecycleSetupFailed, arg)
}

// withDetail is a lifecycle event whose detail is the marker's argument as
// written, absent when it had none.
func withDetail(phase contract.LifecyclePhase, arg string) contract.Event {
	ev := lifecycle(phase)
	if arg != "" {
		var cut Truncation
		detail := cut.Text("detail", arg, contract.MaxTextBytes)
		ev.Lifecycle.Detail = &detail
		ev.Truncated = cut.Fields()
	}
	return ev
}

func lifecycle(phase contract.LifecyclePhase) contract.Event {
	return contract.Event{Kind: contract.KindLifecycle, Lifecycle: &contract.Lifecycle{Phase: phase}}
}
