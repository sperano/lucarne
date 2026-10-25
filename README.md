# lucarne

Live transcripts for coding agents: a read-only viewer for the output of
Kelos agent runs (Claude Code, Codex, OpenCode), first as a conversation,
later as an inspector.

Status: bootstrapped. The transcript contract, runner fixtures, server
skeleton and UI shell are in place; reading runs is not implemented yet.

- Contract (identity, events, limits, deep links, API, SSE/replay):
  [`contract/README.md`](contract/README.md)
- Fixtures and their provenance: [`fixtures/README.md`](fixtures/README.md)
- Layout, gates and rules for contributors: [`AGENTS.md`](AGENTS.md)

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
`web/dist` build; `./lucarne -listen :8080` serves it.

## License

GPL-3.0; see [LICENSE](LICENSE).
