# syntax=docker/dockerfile:1

# Build stage. Pinned to the build platform and cross-compiled from there:
# CGO is off, so a single native build stage can emit any GOOS/GOARCH and
# multi-arch builds never need emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first so they cache independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Set by buildx. Defaulted so a plain `docker build` still works.
ARG TARGETOS=linux
ARG TARGETARCH
# Stamped into core.Version, so `rq --version` and the User-Agent report the
# real version rather than the source default.
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -trimpath \
      -ldflags="-s -w -X github.com/mohamadkrayem/requestCLI/core.Version=${VERSION}" \
      -o /out/rq ./cmd/rq \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
      -trimpath -ldflags="-s -w" \
      -o /out/echoserver ./scripts/echoserver

# Local fixture server used by the smoke suite and by docker-compose. It sits
# before the rq stage on purpose: an untargeted `docker build .` builds the
# final stage, so rq must be last to stay the default.
FROM alpine:3.20 AS echoserver

LABEL org.opencontainers.image.title="rq-echoserver" \
      org.opencontainers.image.description="Local HTTP fixture server for the rq smoke suite." \
      org.opencontainers.image.source="https://github.com/mohamadkrayem/requestCLI"

RUN adduser -D -u 10001 requestcli

COPY --from=build /out/echoserver /usr/local/bin/echoserver

USER requestcli

EXPOSE 8080 8443

# 0.0.0.0 rather than the 127.0.0.1 default: inside a container the loopback
# default would be unreachable from anywhere else on the compose network.
ENTRYPOINT ["echoserver"]
CMD ["-http", "0.0.0.0:8080", "-https", "0.0.0.0:8443"]

# Runtime stage, and the default build target.
FROM alpine:3.20 AS rq

ARG VERSION=dev

LABEL org.opencontainers.image.title="rq" \
      org.opencontainers.image.description="A terminal HTTP client with HTTPie-style syntax." \
      org.opencontainers.image.source="https://github.com/mohamadkrayem/requestCLI" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

# ca-certificates is required: TLS verification is on by default, and without a
# trust store every https request fails with an unknown-authority error.
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 requestcli

COPY --from=build /out/rq /usr/local/bin/rq

# requestCLI is the old binary name, kept as a symlink for one release; the
# argv[0] deprecation notice lives in main.go.
RUN ln -s /usr/local/bin/rq /usr/local/bin/requestCLI

USER requestcli

ENTRYPOINT ["rq"]
CMD ["--help"]
