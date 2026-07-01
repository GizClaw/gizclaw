ARG BASE_IMAGE=gizclaw-go:linux-amd64-base
FROM ${BASE_IMAGE} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=1 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH

WORKDIR /src

COPY . .

RUN npm ci

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -trimpath -o /out/gizclaw ./cmd/gizclaw

FROM scratch AS artifact
COPY --from=builder /out/gizclaw /gizclaw
