# Architecture

requestCLI is a single-binary HTTP client. There is no server, no database and
no persistent state, so most infrastructure concerns do not apply. What matters
here is layering, error handling, and safe network defaults.

## Layers

```
main.go
  └── cmd/          flag definitions and one subcommand per HTTP verb
        └── command/    turns flags into a request, prints the response
              └── requests/   builds and sends the HTTP request
                    ├── formats/    JSON parsing, normalizing, colorizing
                    ├── input/      multipart form and file bodies
                    ├── authentication/  credentials
                    └── response/   decodes, colorizes and renders the response
```

Dependencies point one way, downward. Nothing below `command/` knows about
cobra or about flags.

### cmd/

Defines the persistent flags and builds one subcommand per HTTP verb from a
single table via `newMethodCmd`. Verbs are never hand-written individually —
that duplication previously caused `trace` to send CONNECT and left four
subcommands without argument validation.

### command/

`Options` holds every flag value for one invocation. `Run` assembles the
request; `PrepareInput` reads multi-line JSON documents from stdin for
`--headers` / `--body` using a single shared scanner, so two documents can be
read from one stream.

### requests/

`BaseRequest` accumulates method, URL, headers, cookies, auth and body.
`SendOptions` carries per-invocation transport settings. URL construction goes
through `net/url` rather than string concatenation.

### response/

Reads the body, decompressing gzip, deflate or brotli, then colorizes according
to the response `Content-Type`. The three print flags are a selection set and
combine.

## Error handling

Library packages return errors; only `cmd.Execute` decides to exit. No package
below `cmd/` calls `log.Fatal` or `os.Exit`. This is what makes the code
testable — previously any parse failure killed the process, which also made the
non-JSON body fallback unreachable.

## Network defaults

- TLS certificate verification is **on**; `--insecure` opts out explicitly.
- Every request is bounded by a timeout (`--timeout`, default 30s), plus dial
  and TLS handshake timeouts on the transport.
- Redirects are not followed unless `--redirect` is passed.
- Scheme-less URLs default to https, except loopback hosts.

## Testing

`go test ./...` covers every package except `main`. Each bug fixed in the
review has a named regression test; see the comments marked `Regression:`.

## Not applicable

The standing doctrine's stateless-servers, migrations, idempotency-key,
health-endpoint, queue and cache rules target networked services. This is a
local CLI with no state to lose and no inbound traffic, so those triggers have
not fired and no such layers exist.

## Decision Log

2026-07-25 — Return errors from library packages instead of `log.Fatal` — a
library that exits the process is untestable, and it made the non-JSON body
fallback dead code.

2026-07-25 — TLS verification on by default, opt out via `--insecure` — it was
unconditionally disabled, silently exposing every HTTPS request (and any Basic
Auth credentials on it) to interception.

2026-07-25 — Default scheme-less URLs to https, except loopback hosts — plain
HTTP was the default; loopback is exempted so local development is not broken
by the change.

2026-07-25 — Bound every request with a client timeout plus dial/TLS timeouts —
a bare `http.Client` meant a hung server hung the CLI indefinitely.

2026-07-25 — Build URLs with `net/url` instead of string concatenation — the
old `"?" + query` append produced `?a=1?b=2` whenever params came from two
sources.

2026-07-25 — Generate all nine verb subcommands from one table — nine
near-identical hand-written files were the direct cause of the CONNECT/TRACE
mix-up and the missing argument validators.

2026-07-25 — Use `net/http` method constants rather than string literals —
case-inconsistent literals (`"Delete"`, `"Trace"`) silently never matched.

2026-07-25 — Share one stdin scanner across `PrepareInput` reads — a per-call
`bufio.Scanner` buffered past the first terminator, so `--headers --body`
together dropped the body.

2026-07-25 — Treat `-S`/`-H`/`-B` as a combining selection set — they were
mutually exclusive via if/else, so `-H -B` silently ignored `-B`.

2026-07-25 — Keep the manual `Accept-Encoding` header and hand-written decoders
— letting net/http handle gzip transparently would be simpler but would drop
brotli support, which the tool already advertises.

2026-07-25 — Keep `--secure` and `--Redirect` as deprecated flags — renaming
outright would break existing scripts for no functional gain.
