# Lucarne's local workflow; CI runs the same steps.

staticcheck_version := "v0.8.1"

# Run every gate, as CI does.
default: check

# Install the frontend's pinned dependencies.
install:
    cd web && npm ci

# Frontend gate: typecheck, lint, unit tests, build into web/dist.
check-web:
    cd web && npm run check && npm run lint && npm test && npm run build

# Go gate: formatting, vet, staticcheck, build, race-enabled tests.
check-go:
    test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }
    go vet ./...
    go run honnef.co/go/tools/cmd/staticcheck@{{staticcheck_version}} ./...
    go build ./...
    go test -race ./...

# The frontend first, so the binaries embed it, then the smoke test and the
# browser suite.
check: check-web check-go build
    ./scripts/smoke.sh ./lucarne
    just e2e

# Browser suite: Chromium against the real server over a fake cluster, with
# the last web/dist build embedded.
e2e:
    cd web && npm run test:e2e

# Build the single executable, embedding whatever web/dist holds.
build:
    go build -o lucarne ./cmd/lucarne

# Build the image for both cluster architectures, as the image-build
# pipeline does (needs Docker with buildx); it stays in the build cache.
image version="dev":
    docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION={{version}} .

# Serve the API on :8080 (the UI it embeds is the last build).
dev-api:
    go run ./cmd/lucarne

# Serve the UI with hot reload on :5173, proxying /api to dev-api.
dev-web:
    cd web && npm run dev
