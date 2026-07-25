# Architecture

requestCLI is a single-binary HTTP client. There is no server, no database and
no persistent state, so most infrastructure concerns do not apply. What matters
here is layering, error handling, and safe network defaults.

## Layers

```
main.go
  └── cmd/          flag definitions and one subcommand per HTTP verb
        └── command/    turns flags into a request, renders the result
              ├── core/     builds and sends the request -> Result
              │     ├── formats/         json validation for command-line input
              │     ├── input/           multipart form and file bodies
              │     └── authentication/  credentials
              └── render/   Result -> terminal string
```

Dependencies point one way, downward. Nothing below `command/` knows about
cobra or about flags.

`core` and `render` are siblings and neither imports the other. That separation
is the point of the whole layout: `core` produces structured data, `render`
turns it into text, and any future front-end (a TUI, an editor plugin, a `--json`
mode) attaches to `core` without inheriting a terminal.

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

### core/

`BaseRequest` accumulates method, URL, headers, cookies, auth and body.
`SendOptions` carries per-invocation transport settings only — what to display
is not a transport concern. URL construction goes through `net/url` rather than
string concatenation.

`Send` returns a `Result`: status, protocol, real `http.Header`, the
decompressed body as `[]byte`, and timing. The body is never decoded here.
`core` must not import `render`, `chroma`, or any terminal package.

### render/

`Render(*core.Result, Options) string` is a pure function: no globals, no TTY
probing, no hidden state. Colour is decided once by the caller (via
`render.ColorEnabled`) and passed in as a bool, which is what makes the output
testable without a terminal and re-renderable on resize.

JSON is formatted straight from the raw bytes; everything else with a known
content type goes through a chroma lexer. The three print flags are a selection
set and combine.

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

2026-07-25 — Split `core` (structured `Result`) from `render` (pure function to
text) — the previous design executed and printed in one step and stored
pre-rendered ANSI strings, which no second front-end can consume. Doing this
after a TUI existed would mean writing the TUI twice.

2026-07-25 — Never decode a response body in order to display it — the old path
unmarshalled into `map[string]any` and re-marshalled, which rounded integers
through `float64`, sorted keys alphabetically and collapsed duplicates. The body
shown was never the body received. `tidwall/pretty` formats the raw bytes
instead, in less code.

2026-07-25 — Delete `removeNewLines` — it stripped `\n` out of string *values*,
corrupting description and markdown fields. It was meant to normalize multi-line
stdin, but `scanRequest` already joins those lines, so it was redundant as well
as destructive. `formats.NewJson` now validates with `json.Compact`.

2026-07-25 — Resolve colour once at the boundary and pass it down as a bool —
each renderer used to decide for itself, so JSON honoured the terminal check and
HTML did not; piping HTML emitted raw escape sequences. `NO_COLOR` is now
honoured too.

2026-07-25 — Describe binary bodies instead of printing them — a raw image or
archive dumped to stdout leaves the terminal in a broken state.

2026-07-25 — Sort header keys when rendering — Go map iteration is randomized,
so two renders of one response differed. Snapshot diffing depends on this being
stable.
