package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/sperano/lucarne/internal/e2e/scenario"
	"github.com/sperano/lucarne/internal/kube/kubetest"
)

// controlPrefix is where the control API is served, outside the paths of
// the real server.
const controlPrefix = "/_e2e"

// maxControlBody bounds the lines one write request appends.
const maxControlBody = 8 << 20

// runInfo is how the control API describes a run.
type runInfo struct {
	Name string `json:"name"`
	// Link is the run's deep link, API the path of its snapshot.
	Link string `json:"link"`
	API  string `json:"api"`
}

// writeRequest appends lines to a run's log.
type writeRequest struct {
	Lines []string `json:"lines"`
}

// control is the API that drives the fake cluster:
//
//	GET  /_e2e/runs                     every run, by name
//	POST /_e2e/live                     a new live run
//	POST /_e2e/runs/{name}/write        append {"lines": [...]} to its log
//	POST /_e2e/runs/{name}/break        break its log streams
//	POST /_e2e/runs/{name}/rotate       rotate its log
//	POST /_e2e/runs/{name}/finish?exit=N&phase=P
//	POST /_e2e/runs/{name}/delete       delete its Task, as its TTL does
//	POST /_e2e/streams/cut              end the browser's SSE streams, hold reconnects
//	POST /_e2e/streams/restore          let the held reconnects through
//	GET  /_e2e/streams                  {"open": N}, the SSE streams being served
type control struct {
	world *scenario.World
	gate  *streamGate
}

func newControl(world *scenario.World, gate *streamGate) *control {
	return &control{world: world, gate: gate}
}

func (c *control) register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+controlPrefix+"/runs", c.list)
	mux.HandleFunc("POST "+controlPrefix+"/live", c.live)
	mux.HandleFunc("GET "+controlPrefix+"/streams", c.streams)
	mux.HandleFunc("POST "+controlPrefix+"/streams/cut", c.gated(c.gate.cut))
	mux.HandleFunc("POST "+controlPrefix+"/streams/restore", c.gated(c.gate.restore))
	mux.HandleFunc("POST "+controlPrefix+"/runs/{name...}", c.act)
}

func (c *control) list(w http.ResponseWriter, _ *http.Request) {
	runs := map[string]runInfo{}
	for _, name := range c.world.Names() {
		runs[name] = info(c.world.Run(name))
	}
	writeJSON(w, http.StatusOK, runs)
}

func (c *control) live(w http.ResponseWriter, _ *http.Request) {
	r, err := c.world.AddLive()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info(r))
}

// streamCount is how the control API reports the open streams.
type streamCount struct {
	Open int `json:"open"`
}

func (c *control) streams(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, streamCount{Open: c.gate.count()})
}

// gated answers a request by running change on the stream gate.
func (c *control) gated(change func()) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		change()
		w.WriteHeader(http.StatusNoContent)
	}
}

// act runs the action at the end of the path on the run it names.
func (c *control) act(w http.ResponseWriter, req *http.Request) {
	name, action := splitAction(req.PathValue("name"))
	r := c.world.Run(name)
	if r == nil {
		http.Error(w, "no run "+name, http.StatusNotFound)
		return
	}
	if err := c.do(r, action, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *control) do(r *scenario.Run, action string, req *http.Request) error {
	if r.Pod == nil && action != "delete" {
		return errors.New("the run has no pod")
	}
	switch action {
	case "write":
		var body writeRequest
		if err := json.NewDecoder(io.LimitReader(req.Body, maxControlBody)).Decode(&body); err != nil {
			return err
		}
		r.Pod.Write(body.Lines...)
	case "break":
		r.Pod.Break()
	case "rotate":
		r.Pod.Rotate()
	case "finish":
		code, err := strconv.ParseInt(req.URL.Query().Get("exit"), 10, 32)
		if err != nil {
			return err
		}
		phase := req.URL.Query().Get("phase")
		if phase == "" {
			phase = kubetest.PhaseSucceeded
		}
		return r.Finish(int32(code), phase)
	case "delete":
		return r.Task.Delete()
	default:
		return errors.New("unknown action " + action)
	}
	return nil
}

// splitAction splits "<name>/<action>": a run's name has slashes too.
func splitAction(path string) (string, string) {
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return path, ""
	}
	return path[:i], path[i+1:]
}

func info(r *scenario.Run) runInfo {
	return runInfo{Name: r.Name, Link: r.Link(), API: r.Key.SnapshotPath()}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
