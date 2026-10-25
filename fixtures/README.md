# Runner fixtures

Container output of Kelos agent runs, one file per scenario, for the runner
adapters' tests. Each file is what `kubectl logs` returns for the agent
container without `--timestamps`: the runner's stdout and the entrypoint's
stderr, interleaved, one line per line.

```
<runner>/<version>/<scenario>.log
```

`<runner>` is the Kelos agent type (`claude-code`, `codex`, `opencode`) and
`<version>` the CLI version the output belongs to. [`manifest.json`](manifest.json)
records, for every file, its provenance, the scenarios it covers and its
line counts, plus the versions deployed when the fixtures were written and
the upstream sources each runner's fixtures are based on. `go test
./fixtures/` checks the manifest against the files, that every scenario a
runner must cover is covered at the deployed version or documented under
`notObservable`, and that no file holds private data (`internal/privacy`).

## Provenance

Every fixture is **synthetic** today: no authorized, sanitized capture of a
real run exists yet. They were written from the runners' source at the
deployed versions (Claude Code 2.1.296, Codex 0.162.1, OpenCode 1.19.0, Kelos
v0.60.0); the manifest's `basis` names the files, and each fixture's `notes`
say which details are illustrative and need checking against a capture.
Synthetic fixtures show the shapes the adapters must handle; they are not
evidence of what a run emits, and no output of an adapter test is a claim
about real runs.

What a runner cannot show at its deployed version is listed under
`notObservable` with the reason (OpenCode prints no retries and, as Kelos
starts it, no reasoning; Codex reports no cost and no diff).

## Adding a captured fixture

1. Capture only with the owner's authorization, from a run started for the
   purpose with a fabricated prompt.
2. Sanitize: replace every hostname, URL, repository, path, e-mail address,
   identifier and name that is not fabricated with `example` values
   (`*.example.com`, `github.com/example/...`), and drop anything secret. A
   private transcript never goes in this repository.
3. Add it under the runner and version it came from, with an entry whose
   `provenance` is `captured` and `capturedOn` the capture date, and run
   `go test ./fixtures/`.
