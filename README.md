# lucarne

Live transcripts for coding agents: a read-only viewer for the output of
Kelos agent runs (Claude Code, Codex, OpenCode), as a conversation or, for
the details, as an inspector.

Status: both views are complete and packaged. The transcript contract,
runner fixtures, the three runner adapters, the Kubernetes source, the
snapshot/SSE server, the live conversation and inspector, end-to-end tests
and the container image are in place.

- Contract (identity, events, limits, deep links, API, SSE/replay):
  [`contract/README.md`](contract/README.md)
- Fixtures and their provenance: [`fixtures/README.md`](fixtures/README.md)
- Layout, gates and rules for contributors: [`AGENTS.md`](AGENTS.md)

## Using it

The home page, `/`, lists the runs of the allowlisted namespaces, newest
first, and searches them as you type: every word must appear in a run's
Task name, namespace, Task UID, phase or runner (`?q=` keeps the search in
the address). Enter opens the first match, ↓/↑ move between them, and the
**Lucarne** title of every page leads back to it. It shows the first 100
matches; the server lists at most the 1 000 newest runs, and says how many
older ones it left out.

A run's link, `/runs/{cluster}/{namespace}/{taskUID}` (Gordon's
announcements add `?attempt=` for the pod they start), opens the
**conversation**. The **View** switch above it changes to the **inspector**
and back in place: the same run, snapshot and live stream, with the attempt
and the selected event (`#event=`) kept. `?view=inspector` opens the
inspector directly; a middle-click on either view opens that place in a new
tab. Desktop browsers only: there is no mobile layout.

- **Conversation**: messages, reasoning the runner prints, tool calls with
  their results, edits, errors and lifecycle lines, in order; search,
  follow or pause on a live run, the attempt picker, and a link to any item.
- **Inspector**, in three columns: the run's context (only facts the source
  gave; the rest reads *Unknown*), the attempt's events as a list, and the
  selected event's detail: rendered as in the conversation, its facts
  (attempt, log line, times, exit code, what Lucarne cut), its related
  events (the call, its results, edits, parent and sub-calls), why a result
  is missing, and its log line as the runner wrote it, with copy buttons.
  Filters by kind, by tool and to failures only; the search marks and steps
  through the matches among the events listed. Keys in the list: ↑/↓, Page
  Up/Down, Home/End, Enter to the detail, Escape back.

Limits of the views:

- Each renders a window of 100 items or rows, grown by 100 to at most 600;
  the rest are a button, End or a search away.
- A run holds at most 20 000 events and 32 MiB (the contract's
  [Limits](contract/README.md#limits)); past them its oldest events are
  dropped, and both views say how many. Facts the inspector would read from
  a dropped event say they may be among them.
- The inspector's row summaries are cut to 160 characters. Log lines are
  kept in the server's memory as they are read, each up to 64 KiB, at most
  16 MiB and 20 000 lines per run, oldest dropped; a dropped line or a gap
  answers *not found*.
- Copying needs a secure context (HTTPS); elsewhere the button says the
  clipboard refused.

## Development

Requirements: Go (the version in `go.mod`), Node 22, and
[`just`](https://github.com/casey/just) for the recipes below.

```sh
just install    # npm ci in web/
just check      # every CI gate: web, Go, embedded build, smoke test
```

For UI work, run the API and the Vite dev server side by side:

```sh
just dev-api    # Go server on :8080
just dev-web    # UI on http://localhost:5173, proxying /api to :8080
```

`just build` writes the single executable `./lucarne`, embedding the last
`web/dist` build; `./lucarne -listen :8080` serves it. To read runs, name
the cluster and each namespace it may read
(`-cluster example -namespace agents`); it then uses the in-cluster
credentials, or outside a cluster the kubeconfig's current context.

## Running

### Configuration

Lucarne is configured by flags only; it reads no file and no environment
variable besides the Kubernetes client's (`KUBECONFIG` outside a cluster).

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `:8080` | address to serve HTTP on |
| `-cluster` | empty | name of the cluster on run keys and links (Kubernetes has none); must match Gordon's links |
| `-namespace` | none | a namespace whose runs may be read; repeat it for several. Without one, the run endpoints answer 501 |
| `-drain-delay` | `0s` | after SIGTERM, how long `/readyz` answers 503 before the server stops taking connections |

The bounds (runs followed at once, events and bytes per run, replay, source
lines, slow clients) are fixed; the contract's
[Limits](contract/README.md#limits) and `internal/stream` name them, and
every snapshot publishes them. At most 16 runs are followed at once, each
holding up to 32 MiB of events (replay shares them) and 16 MiB of source
lines: up to 768 MiB of transcript data before the runtime's own, so size
the memory limit above it. Logs go to stderr as JSON.

### Endpoints and probes

| Path | Answers |
| --- | --- |
| `GET /healthz` | `ok` while the process serves: the liveness probe |
| `GET /readyz` | `ready`, or 503 from SIGTERM on: the readiness probe. It calls no source, so a Kubernetes API that does not answer leaves the UI reachable |
| `GET /api/v1/...` | the [HTTP API](contract/README.md#http-api): the contract, the run list, run snapshots, their SSE streams and their events' source lines |
| `GET /`, `/runs/{cluster}/{namespace}/{taskUID}` | the UI: the run search and the [deep links](contract/README.md#deep-links) |

On SIGTERM Lucarne turns unready, waits `-drain-delay`, closes the open
streams and shuts down within 10 seconds. Each browser reconnects; the next
server's history is another, so it gets a reset and loads the run again.

### Container image

The [`Dockerfile`](Dockerfile) builds the frontend with Node, embeds it in a
static Go binary and ships that binary alone on `scratch`, for
`linux/amd64` and `linux/arm64`, with the licenses of what it bundles under
`/licenses`. It runs as UID/GID 65532 and writes nothing, so it runs with:

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities: { drop: [ALL] }
  seccompProfile: { type: RuntimeDefault }
```

Images are built and pinned by the homelab's image-build pipeline from a
reviewed commit of `main`, never tagged by hand. `--build-arg VERSION=<tag>`
names the build in the startup log. `just image` builds both platforms
locally (Docker with buildx).

Lucarne reads with its service account. It needs, in each allowlisted
namespace only: `get` and `list` on `tasks.kelos.dev`, `get` on
`jobs.batch`, `get` and `list` on `pods`, and `get` on `pods/log`
(`internal/kube/doc.go`). It never writes.

### Data exposure

- **Whoever reaches the port reads every run of the allowlisted
  namespaces.** Lucarne has no authentication or authorization of its own:
  serve it only behind the authenticating proxy, and allowlist only the
  namespaces its users may read. A Task UID is an identifier, not a secret.
- **A transcript is everything the agent container printed**: messages,
  reasoning the runner prints, tool inputs and outputs (file contents,
  command output, anything a command echoed, secrets included), Kelos
  outputs (branch, pull request, cost, tokens). Lucarne shows it as the
  source wrote it, within the limits; the inspector also shows each event's
  log line as written, which can hold fields no adapter renders.
- **What it never reads**: the Task's spec (its prompt is dropped when the
  Task is decoded), init and other containers, pods that the Task's Job does
  not control, namespaces outside the allowlist, Secrets. Cluster
  credentials stay in the server; the browser gets only the contract's JSON.
- **In the browser**, log content is rendered as text, never as HTML; only
  `http:`, `https:` and `mailto:` URLs become links. The Content Security
  Policy allows same-origin resources only, so a transcript cannot load a
  remote image or run a script, and no referrer leaves the page.
- **In the service's diagnostics**, runs are named by their link path, and
  errors are logged; log lines are not. Errors of the Kubernetes API are
  logged but never sent to clients, which get fixed messages. The
  end-to-end tests check that no transcript text reaches the log.
- **At rest**, nothing: runs are held in memory while watched (at most 16,
  each bounded by the contract's limits) and dropped after five minutes
  unwatched or on restart.

### Source limitations and expiry

Lucarne reads Kubernetes pod logs and keeps nothing; there is no archive.

- A finished Task is deleted 72 hours after it completes (Kelos
  `ttlSecondsAfterFinished`); its link then answers *no such run, or it
  expired*. A pod can go sooner: eviction, node loss, a deleted Job. A run
  open when its Task is deleted keeps what was read, marked deleted, its
  attempts expired.
- The kubelet keeps the log of a container's current and previous instance
  only: older restarts are listed as expired. Log rotation can remove the
  start of a long log; a re-read that cannot line up with what was read
  shows a gap.
- An attempt's log is read up to 64 MiB, a line up to 1 MiB; past them, a
  gap or a truncated event says so.
- A server restart starts a new history: clients get a reset and load the
  run again from the source, which still has it while the pods exist.

Each of these shows in the UI as such, never as an empty or complete
transcript; [States](contract/README.md#states) defines them.

## Tests

`just check` runs every gate (see [`AGENTS.md`](AGENTS.md#gates)), the
browser suite included: `just e2e` runs Chromium against the real server,
stream and Kubernetes source over a fake cluster
(`internal/e2e/cmd/lucarne-e2e`), with every runner fixture, live,
reconnecting, failed, retried, expired, hostile and oversized runs in both
views, and view switches that must load no second snapshot and leave the
server one stream per page. `go test ./internal/e2e/` checks the same paths
over HTTP.

## License

GPL-3.0; see [LICENSE](LICENSE).
