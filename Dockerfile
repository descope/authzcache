# we use the build platform because we cross compile the go binary for the target platform
# that way we don't use emulation and slow down builds
FROM --platform=${BUILDPLATFORM} golang:1.25 AS builder
RUN go version

ARG repo_name
ARG build_dir=cmd
ARG port=8080
ARG entry_point=app

RUN go env -w GOPRIVATE=github.com/descope/*

COPY . /go/src/github.com/descope/$repo_name
WORKDIR /go/src/github.com/descope/$repo_name

RUN --mount=type=secret,id=github_token,dst=/githubtoken token=`cat /githubtoken` && git config --global url."https://x-access-token:$token@github.com/descope".insteadOf "https://github.com/descope"

RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod download
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod verify
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod tidy -diff
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod vendor

# Cross compile to the target arch
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -o /tmp/$entry_point $build_dir

# Start scratch
FROM scratch AS base

ARG port=8080
ARG entry_point=app
ARG GIT_SHA

ENV GIT_SHA=${GIT_SHA}
WORKDIR /root/
COPY --from=builder --chown=1000:1000 /tmp/$entry_point app
COPY --from=builder --chown=1000:1000 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

EXPOSE $port

USER 1000:1000

ENTRYPOINT ["./app"]
HEALTHCHECK --interval=5s --timeout=10s --start-period=60s --retries=3 CMD ["./app","--healthz"]

# Do prod stuff
FROM base AS prod

