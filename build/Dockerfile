ARG BASE_IMAGE=gizclaw-go:linux-amd64-base
FROM ${BASE_IMAGE} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=dev

ENV CGO_ENABLED=1 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH

WORKDIR /src

COPY . .

RUN npm ci && npm run build:monitor

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -trimpath \
      -ldflags "-X github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo.Version=${BUILD_VERSION} -X github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo.Commit=${BUILD_COMMIT}" \
      -o /out/gizclaw ./cmd/gizclaw

FROM scratch AS artifact
COPY --from=builder /out/gizclaw /gizclaw
