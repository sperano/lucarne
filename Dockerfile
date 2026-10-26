# syntax=docker/dockerfile:1

# The lucarne image: cmd/lucarne with the production frontend embedded, for
# linux/amd64 and linux/arm64, the cluster's architectures. The web and go
# stages run on the build platform and Go cross-compiles for the target, so
# the frontend is built once and neither Node nor Go runs under emulation.
# Node builds the frontend only: the image holds no Node.
#
#   docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=<version> .

# The Node major `just check` and CI use.
FROM --platform=$BUILDPLATFORM node:22.23-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
# No dependency needs an install script (the browser for the tests is
# downloaded by an explicit command, never at install), so none runs.
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts
# The UI validates against the contract's schema, outside web/.
COPY contract/v1/transcript.schema.json /src/contract/v1/transcript.schema.json
COPY web/ ./
# Writes dist/, and dist/.vite/license.md for the bundled dependencies.
RUN npm run build

# The module's go line.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS go
ARG TARGETARCH
# The build this image is made from, logged at startup; the binary reports
# "dev" when it is unset or empty.
ARG VERSION
ENV CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist web/dist
# The frontend must be embedded: without it the binary answers the UI 503.
RUN test -f web/dist/index.html
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
	go build -trimpath -ldflags "-s -w ${VERSION:+-X main.version=$VERSION}" \
		-o /out/lucarne ./cmd/lucarne
# The image distributes the binary, so it carries the licenses of what it
# links.
RUN --mount=type=cache,target=/go/pkg/mod sh scripts/collect-licenses.sh /out/licenses

# scratch: a static binary needs no libc, shell or user database. In the
# cluster it reaches the API server with the service account's CA, so it
# needs no system certificates either.
FROM scratch
ARG VERSION
LABEL org.opencontainers.image.title="lucarne" \
	org.opencontainers.image.description="Live transcripts for coding agents" \
	org.opencontainers.image.source="https://github.com/sperano/lucarne" \
	org.opencontainers.image.licenses="GPL-3.0-only" \
	org.opencontainers.image.version="${VERSION}"
COPY --from=go /src/LICENSE /licenses/lucarne/LICENSE
COPY --from=go /out/licenses /licenses
COPY --from=web /src/web/dist/.vite/license.md /licenses/web/license.md
COPY --from=go /out/lucarne /lucarne
# Numeric, so that Kubernetes' runAsNonRoot can check it without a passwd
# file. The binary writes nothing to disk: the root filesystem can be
# read-only, and it needs no capability.
USER 65532:65532
# cmd/lucarne's defaultListen; -listen changes it.
EXPOSE 8080
# No HEALTHCHECK: scratch has no shell or curl; probe GET /healthz (alive)
# and GET /readyz (taking requests) instead.
# Exec form, so SIGTERM reaches the binary, which drains and shuts down
# gracefully.
ENTRYPOINT ["/lucarne"]
