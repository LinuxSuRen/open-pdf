# open-pdf container image.
#
# Base images are overridable so builds work behind registry mirrors, e.g.:
#   docker build \
#     --build-arg GO_IMAGE=docker.1ms.run/library/golang:1.27-alpine \
#     --build-arg NODE_IMAGE=docker.1ms.run/library/node:22-alpine \
#     --build-arg GOPROXY=https://goproxy.io,direct \
#     -t open-pdf:dev .

ARG GO_IMAGE=golang:1.27-alpine
ARG NODE_IMAGE=node:22-alpine

# ---- Stage 1: build the Vue frontend into static assets --------------------
FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
ARG NPM_REGISTRY=""
RUN if [ -n "$NPM_REGISTRY" ]; then npm config set registry "$NPM_REGISTRY"; fi \
    && npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Stage 2: build the static Go binary with the embedded UI --------------
FROM ${GO_IMAGE} AS build
WORKDIR /src
ARG GOPROXY=""
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X github.com/linuxsuren/open-pdf/internal/buildinfo.Version=${VERSION}" \
      -o /out/open-pdf ./cmd/open-pdf \
    && mkdir -p /out/tmp

# ---- Stage 3: scratch runtime ----------------------------------------------
# The binary is fully static and probes its own /healthz for the Docker
# healthcheck, so no shell, CA bundle or package manager is needed. It runs
# as the unprivileged UID 65534 with a writable /tmp.
FROM scratch
COPY --from=build /out/open-pdf /usr/local/bin/open-pdf
COPY --from=build --chown=65534:65534 /out/tmp /tmp
ENV OPEN_PDF_ADDR=:8080
EXPOSE 8080
USER 65534:65534
HEALTHCHECK --interval=30s --timeout=5s --start-period=3s --retries=3 \
  CMD ["/usr/local/bin/open-pdf", "-self-check"]
ENTRYPOINT ["/usr/local/bin/open-pdf"]
