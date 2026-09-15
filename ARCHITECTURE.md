# Architecture

rq is a single-binary HTTP client. There is no server, no database and
no persistent state, so most infrastructure concerns do not apply. What matters
here is layering, error handling, and safe network defaults.

## Layers

```
cmd/rq/main.go      entry point
  └── cmd/          flag definitions, one subcommand per HTTP verb, and `run`
        └── command/    turns flags + request items into a request, renders the result
              ├── reqitem/  parses HTTPie-style positional request items
              │     └── input/           multipart form and file bodies
              ├── httpfile/ parses .http files and resolves {{variables}}
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

### reqitem/

Parses HTTPie-style positional request items (`key=value`, `key:=value`,
`Key:value`, `key==value`, `key@path`, `key=@path`) into a typed `Items` slice,
and encodes those items as a JSON body, form values, query values, header
operations or multipart parts. This is a CLI-surface grammar, not a transport
concern, so it sits beside `core` rather than inside it: `core` must stay
usable by a front-end that never sees a command line. Only `command` imports
it; `cmd` does not, and `input` does not import it either, so CLI syntax never
sinks below `core`.

### httpfile/

Parses `.http` request files into `httpfile.Request` values and substitutes
`{{variables}}`. Standard library only: it does not import `core`, for the same
reason `reqitem` does not — a request file is a source format, not a transport
concern, and `core` has to stay usable by a front-end that never reads one.

Variable precedence is the order of `Resolver.Sources`, lowest first. Today
there are two (file-level `@vars`, then `--var`); the full model inserts system,
collection, environment and request scopes between them without changing the
resolution logic. `Source.Origin` names where a value came from, which is what
`rq vars` will report.

### core/

`BaseRequest` accumulates method, URL, headers, cookies, auth and body.
`SendOptions` carries per-invocation transport settings only — what to display
is not a transport concern. URL construction goes through `net/url` rather than
string concatenation.

`Send` returns a `Result`: status, protocol, real `http.Header`, the
decompressed body as `[]byte`, and timing. The body is never decoded here.
`core` must not import `render`, `chroma`, or any terminal package.

A `text/event-stream` response — or any response when `SendOptions.Stream` is
set — comes back with `Result.Stream` populated and `Result.Body` nil. The two
are never both present: a stream has no complete body to hand over. The caller
then pulls `core.Event` values from `Stream.Next` until `io.EOF` and must
`Close` the result, because `Send` can no longer close the body itself.
`Result.Close` is safe on any result, repeatable, and safe to call
concurrently — the CLI closes from a signal handler to unblock a parked read.

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
as destructive. `formats.NewJSON` now validates with `json.Compact`.

2026-07-25 — Resolve colour once at the boundary and pass it down as a bool —
each renderer used to decide for itself, so JSON honoured the terminal check and
HTML did not; piping HTML emitted raw escape sequences. `NO_COLOR` is now
honoured too.

2026-07-25 — Describe binary bodies instead of printing them — a raw image or
archive dumped to stdout leaves the terminal in a broken state.

2026-07-25 — Sort header keys when rendering — Go map iteration is randomized,
so two renders of one response differed. Snapshot diffing depends on this being
stable.

2026-07-25 — Parse request items in a new reqitem package rather than in core or command — the syntax is a CLI-surface grammar with a dozen ambiguity cases, so it needs unit tests without an Options struct, and core must stay usable by a TUI that never sees a command line.

2026-07-25 — Fix the URL at positional argument 0 and never item-parse it — it removes the whole Key:value vs scheme-less host:port ambiguity class at zero cost, where HTTPie's position-free items require heuristics.

2026-07-25 — Request items override -n/--headers and -q for the same key, and conflict with -b/--body — items are the more explicit source and Key: must be able to unset what a flag set, while merging a hand-written JSON body with generated fields would require decoding and re-encoding it, reintroducing the key reordering M1.5 removed.

2026-07-25 — Build the item JSON body by writing bytes in order rather than through map[string]any — the same reason display never decodes: a map sorts keys and rounds large integers, so age:=1234567890123456789 would not reach the server intact.

2026-07-25 — A key@file item implies multipart; combining it with -f is an error — inferring the encoding matches HTTPie and the alternative silently drops the file.

2026-07-25 — Capture the sent request as core.SentRequest on Result and render it from render.Request — -v is a display concern, and printing from inside Send would make the request invisible to every other front-end.

2026-07-25 — Adopt HTTPie's 3/4/5 exit codes for --check-status and add 2 for transport failure — a script today cannot tell "server said 404" from "could not reach the server", which is the whole reason --check-status exists.

2026-07-25 — Read piped stdin as the body only when no explicit body source is given, with --ignore-stdin as the escape hatch — explicit beats implicit, and an empty pipe or /dev/null must behave exactly as today.

2026-07-25 — Rename the binary to rq and ship requestCLI as a symlink with an argv[0] deprecation notice for one release — a second main package would duplicate the entry point for no gain.

2026-07-25 — Keep the module path github.com/mohamadkrayem/requestCLI — renaming it is a breaking import-path change with no user-visible benefit in M1.

2026-08-19 — Add a .dockerignore rather than leaving `COPY . .` unfiltered — the
build context carried `.git`, the host-built `rq` binary and the gitignored
local notes (`VISION.md`, `M1-BRIEF.md`), which invalidated the layer cache on
every local build and put local-only files inside an image layer.

2026-08-19 — Make `core.Version` a var so release and container builds can stamp
it with `-ldflags -X` — a const meant every image reported the source default
regardless of the tag it was built from.

2026-08-19 — Cross-compile in a `$BUILDPLATFORM` stage instead of building under
emulation — CGO is already off, so multi-arch images cost one extra `go build`
rather than a qemu-emulated toolchain.

2026-08-19 — Ship the fixture echoserver as a second image target and a compose
service — the smoke suite already depends on it, and a container image needs an
end-to-end check that does not reach a real API.

2026-08-19 — Keep `scripts/smoke.sh` on a locally built binary and give the image
its own smaller `scripts/docker-smoke.sh` — the full suite needs local files for
upload items and a closed local port for the transport-failure checks, neither
of which survives containerisation cleanly; the image check covers only what can
break in the image (non-root user, trust store, stdin, argv[0] alias).

2026-09-10 — Add `cmd/rq` as the canonical entry point and keep the root main
package for one release — `go install` names a binary after the last element of
its import path, so installing the module root could only ever produce
`requestCLI`, which nags on every run and has no `rq` to switch to. This reverses
the 2026-07-25 entry that rejected a second main package; the argv[0] notice
moves into `cmd.Execute` so the two entry points share it rather than duplicate it.

2026-09-12 — Package releases with a checked-in `scripts/release.sh` plus a
tag-triggered workflow, rather than GoReleaser — the script reproduces the
v1.2.0 archive layout exactly (including the `requestCLI -> rq` symlink), needs
nothing beyond Go and coreutils to rehearse a tag locally, and keeps the
reproducibility guarantees explicit and inspectable. GoReleaser would also work;
revisit it if the project ever wants Homebrew, Scoop or Nix manifests, which it
generates nearly for free and this script would not.

2026-09-12 — Make the release archives byte-reproducible — pinned owner, sorted
entries, an mtime from `SOURCE_DATE_EPOCH`, `gzip -n` and `zip -X`. Publishing
`SHA256SUMS` is only worth something if a third party can rebuild the tag and
regenerate the same digests; otherwise it protects against a corrupted download
and nothing else.

2026-09-12 — Fail the release when the git tag disagrees with `core.Version` —
`go install` applies no link flags, so a module-path install reports the source
default no matter what the tag says. A forgotten bump is otherwise invisible
until the tag is public and someone reports the wrong `--version`.

2026-09-12 — Run vet, tests and the smoke suite inside the release job — the CI
workflow triggers only on `master` and pull requests, so until now a tag push
published binaries that nothing had verified.

2026-09-12 — Create the release as a draft rather than publishing it — every
release so far has carried a hand-written upgrade guide, and `--generate-notes`
would replace that prose with a commit list. The workflow assembles and attaches
the artifacts; a human writes the notes and presses publish.

2026-09-12 — Remove the `requestCLI` binary name in v1.3.0 — it shipped as a
deprecated alias for all of v1.2.0 with an on-every-run stderr notice, and the
README, the release notes and the notice itself all promised removal in the next
release. This retires the argv[0] notice in `cmd.Execute`, the root `main`
package, the symlink in the release archives and the one in the image. The
module path stays `github.com/mohamadkrayem/requestCLI`: renaming it is a
breaking import-path change that buys nothing.

2026-09-12 — Model a stream as `Result.Stream` alongside a nil `Body`, rather
than turning `Body` into an `io.Reader` — every existing caller and both
renderers consume `[]byte`, and making them all handle a reader in order to
serve one new response type would push buffering into each of them. A front-end
checks one field to know which kind of result it has.

2026-09-12 — Bound a streamed request with a cancellable timer instead of
`http.Client.Timeout` — that field also covers reading the body and cannot be
lifted once the response turns out to be a stream, so the default 30s would cut
off every long-lived stream. `--timeout` now bounds connect and headers for a
stream, and still bounds the whole exchange for a buffered response. The visible
cost is that a timeout no longer reports Go's "Client.Timeout exceeded"; it
names the configured timeout instead, which is clearer anyway.

2026-09-12 — Surface keepalive comment frames as events carrying `Comment:
true`, instead of swallowing them in the parser — `--raw` exists to show
framing, and a parser that drops comments makes it unable to. Returning them
immediately also bounds memory, which accumulating them into the next event
would not.

2026-09-12 — Render a stream through a pure `render.Event` per event rather than
letting `core` print — it keeps the constraint that `core` never touches a
terminal, and it is what lets a TUI re-render a scrollback of events on resize.
`command` owns the loop, the signal handling and the writer.

2026-09-12 — Compact JSON event payloads onto one line instead of
pretty-printing them — a stream is read as a sequence, and expanding each frame
over a dozen lines hides the sequence it exists to show. Key order and integer
precision are still preserved exactly, for the same reason the body renderer
preserves them.

2026-09-12 — Print events on stdout and the summary on stderr — piping a stream
into a parser must yield events and nothing else, and the summary is diagnostic
output about the exchange rather than part of it.

2026-09-12 — Parse `.http` files in a new `httpfile` package that imports only
the standard library — a request file is a source format, not a transport
concern. Keeping it out of `core` is the same rule that keeps `reqitem` out:
`core` must stay usable by a front-end that never reads a file, and a grammar
with this many edge cases needs to be testable without building a request.
`command` maps the parsed request onto `core.BaseRequest`.

2026-09-12 — Adopt the existing `.http` dialect rather than inventing a format —
it is already read by VS Code REST Client, the JetBrains HTTP Client and
kulala.nvim, so files move in both directions and every one of those editors is
an on-ramp. The parser therefore tolerates constructs it does not implement
(`# @ignore`, `# @snapshot`) instead of rejecting them, which is what lets the
format be extended later without forking it.

2026-09-12 — Express variable precedence as an ordered list of `Source` values
rather than branching — the eventual model has eight scopes (system file,
collection defaults, nested collection, environment, file, request, command
line, plus built-ins), and each one becomes a Source inserted at the right
index with nothing else changing. `Source.Origin` is on the interface from the
start because `rq vars` has to report where a value came from, and that is
impossible to add afterwards without touching every source.

2026-09-12 — Substitute variables in a single pass — a value that itself
contains `{{…}}` is left alone. Recursive expansion invites cycles and lets an
injected value quietly become a reference, and no other client of this format
resolves recursively.

2026-09-12 — Report an unresolved variable as an error naming it, rather than
leaving `{{base_url}}` in the URL — otherwise the failure surfaces much later
as a confusing DNS or connection error against a literal brace.

2026-09-12 — Report positions as `file:line:` — editors and terminals turn that
form into a clickable jump, which matters more than prose for a feature whose
whole subject is a file the user is editing.

2026-09-12 — Run the requests in a file in order and stop at the first failure —
a file is usually a sequence (log in, then use the token), and continuing past
a broken step produces a cascade that hides the real error.

2026-09-12 — Print per-request headings on stderr, like the stream summary — a
piped run must carry response bodies and nothing else.
