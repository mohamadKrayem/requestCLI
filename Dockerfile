# Build stage
FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first so they cache independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rq .

# Runtime stage
FROM alpine:3.20

# ca-certificates is required: TLS verification is on by default.
RUN apk add --no-cache ca-certificates

COPY --from=build /out/rq /usr/local/bin/rq

# requestCLI is the old binary name, kept as a symlink for one release; argv[0]
# deprecation notice lives in main.go.
RUN ln -s /usr/local/bin/rq /usr/local/bin/requestCLI

# Run as a non-root user.
RUN adduser -D -u 10001 requestcli
USER requestcli

ENTRYPOINT ["rq"]
CMD ["--help"]
