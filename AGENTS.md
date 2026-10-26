# AGENTS.md

Instructions for anyone, human or agent, changing this repository.

## What Lucarne is

Lucarne is a read-only viewer for the transcripts of Kelos coding agents: it
reads a run's pod logs (Claude stream-json, Codex `exec --json`, OpenCode
`run --format json`), normalizes them into one event stream and renders
them live in the browser, first as a conversation, later as an inspector.

Lucarne observes; it never takes part. It does not start, steer or stop
agents, and nothing in the agent pipeline waits on it.

Status: bootstrapped. The contract, the fixtures, the server, the Claude
Code, Codex and OpenCode adapters, the Kubernetes log source, the
snapshot/SSE endpoints that wire them together and the live conversation
view exist; the inspector does not yet. Started without `-namespace`, the
server reads no cluster and its run endpoints answer 501.

## What lives here, and what does not

| Repository | Holds |
| --- | --- |
| `sperano/lucarne` (here) | The transcript/API contract, the runner adapters, the log source, the server, the frontend, tests, fixtures, CI and the container build. |
| `sperano/hollingsworth` | Everything that deploys or wires Lucarne into the cluster: Helm chart and values, ApplicationSet entry, RBAC, Gateway and Authentik, the image pin, Gordon's announcement links, and the Kelos onboarding of this repository itself (`helm/kelos/values-hollingsworth.yaml`, including `vikunjaSpawners.prReadiness.ciChecks`). |

## Layout

Go at the repository root, the Svelte/TypeScript frontend in `web/`, as the
agent stacks this repository is onboarded with (`go-service`, then `web`)
expect. One executable, no database.

| Path | Holds |
| --- | --- |
| `contract/` | The versioned transcript/API contract: `README.md` (the specification: identity, events, states, limits, deep links, HTTP and SSE/replay), `v1/transcript.schema.json`, its valid and invalid examples, and the Go types and `Validate`. |
| `fixtures/` | Sanitized, provenance-labelled runner output per runner and version, with `manifest.json`; see its `README.md`. |
| `cmd/lucarne/` | The executable. |
| `internal/adapter/` | Runner-neutral normalization of one attempt's log lines: event IDs and parts, the contract's bounds, raw fallback, the Kelos marker lines. `claude/` is the Claude Code stream-json runner, `codex/` the Codex `exec --json` runner and `opencode/` the OpenCode `run --format json` runner, each with golden tests over its fixtures (`go test ./internal/adapter/<runner>/ -update` rewrites them) and its mapping and capability gaps in `doc.go`; `adaptertest/` holds what their tests share. |
| `internal/kube/` | The Kubernetes source: resolves a run key to a Kelos Task by UID within the configured cluster and namespace allowlist, lists the attempts of its `kelos-agent` container through the Task → Job → pod controller chain (by UID), and reads their logs bounded, re-checking that chain before each stream, following and resuming across breaks. Read-only (get, list, pods/log); its RBAC and limits are in `doc.go`. Tests run against client-go fakes. |
| `internal/stream/` | The live state of the runs being viewed: follows a run's Task, attempts and logs through the source and the adapters, numbers every update, keeps bounded replay and retained events, re-reads broken logs without repeating lines, and serves snapshots and subscriptions with resets, gaps and backpressure; see `doc.go`. `streamtest/` is the scripted source its tests and the server's share. |
| `internal/server/` | HTTP: `/healthz`, `/api/v1/...` (the snapshot and the SSE stream over `internal/stream`), the embedded UI and its deep links, security headers. |
| `internal/privacy/` | The scan that keeps private data out of fixtures and examples. |
| `web/` | The UI (Vite, Svelte 5, TypeScript). `web/src/lib/contract.ts` mirrors the contract; `web/embed.go` embeds `web/dist`. `web/src/conversation/` is the conversation view, over the pure modules of `web/src/lib/` (its model, the Markdown parser, safe links, item windows, search, and `live.ts`, which follows a run's snapshot and SSE stream into the bounded `runstore.ts`; `signin.ts` keeps the complete address across a proxy's sign-in); log content is only ever rendered as text, never with `{@html}`. `web/src/dev/` wraps the adapters' golden files in fabricated snapshots, and fakes the snapshot endpoint and `EventSource`, for the tests and for `npm run dev`'s `/fixtures.html` preview, which the build leaves out. |
| `scripts/smoke.sh` | Starts a built binary and checks the UI, an asset, a deep link and the contract endpoint. |

A contract change touches the schema, the Go types, `web/src/lib/contract.ts`
and an example in one pull request; both test suites fail otherwise.

## Gates

`just check` runs every gate locally, as three CI jobs will once
`.github/workflows/ci.yml` is added (agent tokens cannot push workflow
files; the owner adds or changes them).

| Job | Steps |
| --- | --- |
| `web` | in `web/`: `npm ci`, `npm run check` (svelte-check), `npm run lint` (Prettier, ESLint), `npm test` (Vitest: `*.test.ts` in Node, component tests `*.svelte.test.ts` in headless Chromium, which `npx playwright install chromium` provides for the pinned `playwright`), `npm run build` |
| `go` | `gofmt -l .` empty, `go vet ./...`, staticcheck v0.8.1, `go build ./...`, `go test -race ./...` |
| `app` | after `web`: `go build` with the built UI embedded, then `scripts/smoke.sh` |

`go build` works without a frontend build (the binary then answers UI
requests with 503). `go.mod` ignores `web/node_modules`, which can contain Go
files.

## Tasks that target another repository

A Kelos agent working on a task from the Lucarne project gets a clone of this
repository and nothing else: the spawner routes by project, never by what the
task says. So:

- Commit only Lucarne's own code and documentation here. Never copy Helm,
  OpenTofu, RBAC or Kelos configuration into this repository to "stage" it.
- A task whose work is in `sperano/hollingsworth` belongs in the
  Hollingsworth project, where its agent gets that checkout. If one reaches
  you here, change nothing, reply on the task that it has to move, and stop.
- When a Lucarne change needs a Hollingsworth change too (a new CI check
  name for `prReadiness.ciChecks`, a new image, a new configuration value),
  finish the Lucarne part and name the Hollingsworth change in the PR body
  under what was left out; the owner turns it into a Hollingsworth task.

## Pull requests and reviews

- Agent branches are `kelos/vikunja-<task id>`. Open the PR as soon as the
  gates pass; do not review your own diff.
- Every PR is reviewed by `pr-challenger`; a PR that changes Go files
  (`*.go`, `go.mod`, `go.sum`) also gets `idiomatic-go`, and one that changes
  TypeScript, JavaScript, Svelte, `package.json`, `package-lock.json` or
  `tsconfig*.json` gets `idiomatic-typescript`. Each posts one PR review and
  a `review/<reviewer>` check.
- `main` requires `review/pr-challenger`. A blocking verdict, and a
  suggestions verdict on a PR labelled `kelos:fix-suggestions`, starts a
  follow-up on the PR branch, for a capped number of rounds.
- The owner merges. Merging closes the Vikunja task.

## Rules

- **The repository is public.** Fixtures, examples, tests, comments and docs
  use fabricated or sanitized data only: no private transcripts, prompts,
  tokens, real task titles, Vikunja ids or internal hostnames
  (`*.example.com` instead).
- **Sources are read-only.** No code writes to Kubernetes, Vikunja or
  GitHub, and credentials are requested with read scopes only. Cluster
  credentials never reach the browser.
- **A run is identified by cluster + namespace + Task UID**, an attempt by
  pod UID, container and restart: never by Task name, which a retry or a
  later Task reuses.
- **Render only what the source emitted.** No invented prompts, reasoning,
  timing, costs or outcomes; missing, truncated and expired data is shown as
  such.
- Named constants, not magic numbers.
- Files ≤ ~500 lines, functions ≤ ~50; compose small functions.
- Tests live in sibling files: `foo.go` → `foo_test.go`, `foo.ts` →
  `foo.test.ts`, a component `Foo.svelte` → `Foo.svelte.test.ts`.
