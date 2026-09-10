# requestCLI — Repository Review

**Date:** 2026-07-25
**Reviewed commit:** `c25275f` (master, clean tree)
**Toolchain:** Go 1.22.2 · module declares `go 1.20`

> **Status: addressed on branch `fix/code-review-findings`.**
> This document records the state of the code *before* those fixes. Every item
> in §1–§9 is resolved except where noted below; see `ARCHITECTURE.md` for the
> Decision Log and the `Regression:` comments in the test suite.
>
> Deliberately not done: §1.3 (interactive password prompt — a new feature, not a
> fix) and §5.3 (dropping the manual `Accept-Encoding`, which would remove
> brotli support). Both are recorded in the Decision Log.

## Executive summary

The project builds cleanly (`go build ./...`) and passes `go vet ./...` with zero
findings. The architecture is sensible for a CLI of this size: a shared
`command.Command` carries user input, `requests.BaseRequest` builds and sends,
`response.Response` renders. The colorized JSON/HTML output and the
gzip/deflate/brotli decoding are genuinely nice touches.

That said, `go vet` being clean is misleading. **There are zero tests**, and I
confirmed **six runtime panics and two silent-wrong-behavior bugs by actually
running the binary**. The most severe issue is not a crash but a security
default: TLS certificate verification is disabled unconditionally for every
request.

Verdict: solid bones, but the error-handling strategy (`log.Fatal` in library
code, `println` on errors, unchecked string indexing) is the single root cause
behind most of the defects below. Fixing that pattern fixes most of the list.

---

## 1. Critical — security

### 1.1 TLS verification is disabled for every request, with no way to enable it

`requests/base_request.go:215-219`

```go
Transport: &http.Transport{
    TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
},
```

This is hardcoded. Every HTTPS request this tool makes is silently vulnerable to
a man-in-the-middle attack, and the user is never told. The comment above it
even acknowledges the risk, but there is no flag to turn it off.

For an HTTP client whose whole purpose is inspecting responses — and which also
sends Basic Auth credentials and cookies — this is the most serious problem in
the repo.

**Fix:** default to verification **on**; expose opt-out as an explicit
`-k` / `--insecure` flag (curl's convention), and print a warning to stderr when
it is used.

### 1.2 Plain HTTP is the default

`cmd/root.go:64` — `--secure`/`-s` defaults to `false`, so `requestCLI get
example.com` sends over `http://`. In 2026 the default should be HTTPS with an
opt-out (`--http`), not the reverse. Combined with 1.1, credentials passed via
`--auth` go out over cleartext HTTP by default.

### 1.3 Credentials on the command line

`--auth username=x,password=y` puts the password into shell history and into
`ps` output for any user on the machine. Worth supporting an interactive
password prompt (`--auth user` → prompt) or reading from an env var.

---

## 2. Critical — confirmed crashes

Each of these was reproduced against the built binary.

| # | Trigger | Location | Result |
|---|---------|----------|--------|
| 2.1 | `requestCLI put` (no URL) | `command/parentCMD.go:40` | `panic: index out of range [0] with length 0` |
| 2.2 | `requestCLI head` (no URL) | `command/parentCMD.go:40` | same panic |
| 2.3 | Blank line inside `--body` input | `command/parentCMD.go:106` | `panic: index out of range [-1]` |
| 2.4 | `--multi -b '{"a":1}'` (1-char field name) | `input/multipart_input.go:41` | `panic: slice bounds out of range [:2] with length 1` |

**2.1 / 2.2 — missing arg validation.** `getCmd`, `postCmd`, `delCmd`,
`ConnectCmd` and `TraceCmd` all declare `Args: cobra.MinimumNArgs(1)`.
`headCmd`, `optionsCmd`, `putCmd` and `patchCmd` **do not** — so `args[0]` in
`Command.Run` panics. One-line fix per file.

**2.3 — `scanRequest` indexes without a length check.** `strTest[len(strTest)-1]`
at `command/parentCMD.go:106` panics on any empty line. The same function also
unconditionally appends `"}"` to the collected input (`:120`), which corrupts any
top-level-array body and any input the user already closed properly.

**2.4 — `key[:2]` without a length guard.** `input/multipart_input.go:41` slices
every key to check the `@!` file prefix. Any field name shorter than 2
characters panics. Use `strings.HasPrefix(key, "@!")`.

**Unchecked indexing also appears at:**
- `formats/json_class.go:141` — `isArray` does `js[0]` on a possibly-empty string
- `formats/json_class.go:149` — `removeNewLinesFromJSONString` does `jsonStr[0]`
- `input/multipart_input.go:63,66` — `generateLocation` does `location[0]`

---

## 3. High — wrong behavior

### 3.1 `trace` sends a CONNECT request

`cmd/trace.go:24`

```go
trace_command.Method = "CONNECT"   // copy-paste from conn.go
```

The `trace` subcommand does not send TRACE. Confirmed: a `trace` invocation
against a local test server never reaches the handler at all, because Go's HTTP
client handles CONNECT specially. Should be `"TRACE"`.

### 3.2 Query parameters produce a malformed URL

Confirmed against a local server:

```
$ requestCLI get 127.0.0.1:8099 -q a=1 -f -b '{"b":2}'
SERVER SAW: method=GET uri=/?a=1?b=2      # should be /?a=1&b=2
```

`GenerateUrl` (`requests/base_request.go:69-72`) appends `"?" + query`, and then
`AddQueryString` (`:77-79`) appends **another** `"?" + query`. The second one
must use `&` when the URL already has a query string — or better, both should go
through a single `url.URL`/`url.Values` construction.

### 3.3 The "not JSON → send as text/plain" fallback is dead code

`requests/base_request.go:183-191` is written to catch a non-JSON body and
downgrade it to `text/plain`. It can never run, because
`js.ToMapOptionalJS` calls `log.Fatal` **before** returning the error
(`formats/json_class.go:124`). Confirmed:

```
$ requestCLI get example.com -b 'hello'
2026/07/25 14:46:42 Error in your json format     # process exits; no fallback
```

### 3.4 Method comparisons use inconsistent casing

`requests/base_request.go:183`

```go
if req.Method == "GET" || req.Method == "Delete" || req.Method == "HEAD" ||
   req.Method == "Trace" || req.Method == "Connect" || req.Method == "Options" {
```

Methods are assigned uppercase (`"DELETE"`, `"TRACE"`, `"OPTIONS"`,
`"CONNECT"`), so `"Delete"`, `"Trace"`, `"Connect"` and `"Options"` never match.
DELETE is caught only by the separate check at `:165`. TRACE/CONNECT/OPTIONS
bodies are therefore handled on the wrong branch. Use `http.MethodGet` and
friends from `net/http` instead of string literals.

### 3.5 `-f` silently discards user headers

`command/parentCMD.go:43-46`

```go
if *command.Form {
    *command.HeadersJS = make(map[string]string)   // wipes -n headers
    (*command.HeadersJS)["Content-Type"] = "application/x-www-Form-urlencoded"
}
```

`requestCLI post example.com -f -n X-API-Token=123` loses `X-API-Token`. Set the
key on the existing map instead of replacing it. (Also note the non-canonical
capital `F` in `x-www-Form-urlencoded`.)

### 3.6 `WithHeaders` replaces the header map rather than merging

`requests/base_request.go:132` — `req.Headers = jsonMap` discards anything set
earlier. It should merge, matching `WithHeadersMap`'s behavior at `:137-142`.

### 3.7 `float32` query parameters are silently dropped

`requests/base_request.go:103-105`

```go
case float32:
case float64:
    query.Set(key, strconv.FormatFloat(...))
```

Go does not fall through. The empty `float32` case swallows the value.

### 3.8 `removeNewLinesRecursively` does nothing

`formats/json_class.go:190-206` — the `case string:` branch assigns to the local
parameter `jsonObj`, which is discarded on return. Strings inside the map are
never modified. The function is a no-op for its stated purpose, and its two
callers (`:158`, `:176`) rely on it.

### 3.9 URL scheme detection uses `Contains` instead of `HasPrefix`

`requests/base_request.go:58-59` — a URL like
`example.com/redirect?to=http://evil.com` is treated as already having a scheme
and is never prefixed, producing an invalid request URL.

---

## 4. High — error handling strategy

**13 `log.Fatal` calls sit in library packages** (`requests`, `formats`,
`response`, `command`). This is the root cause of §3.3 and makes the code
untestable — a unit test that hits any of these kills the test binary.

```
requests/base_request.go:129,162,231
formats/json_class.go:28,36,124
response/response_class.go:149,156,162,169,181,186
command/parentCMD.go:70
```

Library code should return errors; only `main`/`cmd` should decide to exit.

**Related: `println(err)` at `requests/base_request.go:250`** uses the *builtin*
`println`, which prints the interface's internal representation rather than the
message. Confirmed output on a connection failure:

```
(0xd61660,0xc0002638c0)
2026/07/25 14:48:11 error in sending the request !!!
```

A user gets a pointer pair instead of "connection refused". Use
`fmt.Fprintln(os.Stderr, err)`.

**Also:** error messages throughout are unhelpful (`"error with your json !!!"`)
— they don't say which field, which line, or what was expected.

---

## 5. Medium — reliability

### 5.1 No timeouts anywhere

`requests/base_request.go:203` creates a bare `&http.Client{}` with no `Timeout`
and no per-dial/TLS/response-header timeouts on the transport. A hung server
hangs the CLI forever with no way out but Ctrl-C. This is the one item here that
directly violates the standing doctrine's "timeouts on every network call, no
unbounded waits."

**Fix:** `Timeout` on the client plus a `--timeout` flag; set `DialContext`,
`TLSHandshakeTimeout`, and `ResponseHeaderTimeout` on the transport.

### 5.2 The client is allocated twice

`requests/base_request.go:203-205` — `client := &http.Client{}` is immediately
overwritten by `client = &http.Client{...}`. Harmless, but dead code.

### 5.3 Manual `Accept-Encoding` defeats Go's transparent decompression

`requests/base_request.go:263-265` sets `Accept-Encoding: gzip, deflate, br`
explicitly. Setting this header manually disables `net/http`'s automatic gzip
handling, which is why `response/response_class.go` has to reimplement gzip,
deflate and brotli by hand. That code works, but it is ~60 lines that mostly
wouldn't be needed if the header were left alone (brotli aside).

### 5.4 `Content-Type: application/json` is sent even on bodyless GETs

`requests/base_request.go:260-262` — every request gets a JSON content type
whether or not it has a body. Should only be applied when a body is present and
no type was inferred.

### 5.5 Double `Body.Close()`

`Send` defers `resp.Body.Close()` (`:254`) and `readResponseBody` also defers
`res.Body.Close()` (`:141`). Harmless with `net/http` today, but sloppy — pick
one owner.

### 5.6 Cookie values are not escaped

`requests/base_request.go:245` passes raw values into `http.Cookie{Value: ...}`.
Values containing `;` or whitespace will produce a malformed header.

---

## 6. Medium — missing infrastructure

| Missing | Impact |
|---|---|
| **Any tests at all** | All 8 packages report `[no test files]`. Every bug in §2–§3 would have been caught by a basic table test. This is the highest-value gap in the repo. |
| **CI** | No `.github/workflows/`. Nothing runs `build`/`vet`/`test` on push. |
| **`Makefile` or task runner** | No standard `make build` / `make test` / `make lint`. |
| **`golangci-lint` config** | `go vet` is clean but a real linter would flag the dead cases, unused code, and shadowing. |
| **`Dockerfile`** | README line 19 promises "Docker container image will be available soon" — still absent. |
| **Release automation** | No GoReleaser config; no prebuilt binaries, so `go install` is the only install path. |
| **`CONTRIBUTING.md`** | `CODE_OF_CONDUCT.md` exists but there's no contribution guide it can point to. |
| **`.gitignore` coverage** | Only ignores `requestCLI`. Should also cover `dist/`, `*.test`, `coverage.out`, `.env`. |

---

## 7. Medium — dead and duplicated code

### 7.1 The seven request subtypes are entirely unused

`requests/get_request.go`, `post_request.go`, `put_request.go`,
`patch_request.go`, `delete_request.go`, `head_request.go`,
`options_request.go` each declare `type XRequest struct { BaseRequest }` and are
never referenced anywhere. Either build the per-method behavior onto them or
delete all seven files.

### 7.2 `input/basic_input.go` is unused

`BaseInput` is declared and never used. The file's only import block is fully
commented out.

### 7.3 The `cmd/` package is ~40 lines of copy-paste × 9

`cmd/get.go`, `post.go`, `put.go`, `patch.go`, `del.go`, `head.go`,
`options.go`, `conn.go`, `trace.go` are near-identical, differing only in the
method string and command name. This duplication is *directly* what caused §3.1
(trace sending CONNECT) and §2.1/2.2 (four files missing `Args:`).

**Fix:** one factory —

```go
func newMethodCmd(use, method, short string) *cobra.Command
```

— called nine times from a table. This collapses ~360 lines to ~40 and makes the
whole class of copy-paste bug impossible.

### 7.4 Misleading names and leftovers

- `authentication.NewBaseRequest` constructs a `BaseAuth`, not a request — should be `NewBaseAuth`.
- `formats` package is named `package json` (`formats/json_class.go:1`), shadowing the stdlib name at every import site and forcing aliases everywhere.
- `requests.NewRequest` has the doc comment `// NewBaseRequest creates a new BaseRequest object` (`:33`).
- Commented-out test scaffolding at `response/response_class.go:39-45`, dead `rootCmd.Flags().Bool("toggle", ...)` at `cmd/root.go:63`, commented-out `Run` block at `cmd/root.go:46-50`.
- `_ = jsonArrayOfMaps` / `_ = modifiedJSONStr` no-op assignments at `formats/json_class.go:152,161`.

### 7.5 Declared-but-unimplemented flags

`cmd/root.go:77-78` — `--text`/`-t` and `--reqHeaders`/`-r` are registered and
appear in `--help`, but nothing reads them. They are marked
`// to be implemented`. Either implement or remove them; shipping non-functional
flags in help output is worse than not having them.

---

## 8. Low — dependencies and toolchain

- **`go.mod` declares `go 1.20`** — out of support. Bump to a current release.
- **`io/ioutil` is deprecated** — used at `response/response_class.go:9,154,160,167`. Replace with `io.ReadAll`.
- **`github.com/alecthomas/chroma v0.10.0`** — v1 is superseded; upstream is on `chroma/v2`.
- **`github.com/spf13/cobra v1.6.1`** — several releases behind.
- **`github.com/dsnet/compress v0.0.1`** — a `v0.0.1` pin for brotli decoding. `github.com/andybalholm/brotli` is the more actively maintained choice.
- **`// direct` comments in `go.mod`** (lines 6-10) are non-standard; `go mod tidy` writes `// indirect` only. Running `go mod tidy` will normalize this.

---

## 9. Low — documentation

`README.md` has drifted from the code:

1. **`delete` is documented but does not exist.** README line 119 shows
   `requestCLI delete example.com`; the command is registered as `del`.
   Confirmed: `Error: unknown command "delete" for "requestCLI"`.
2. **`go install` is missing `@latest`** (line 16). As written it fails outside a
   module.
3. **Undocumented commands:** `head`, `options`, `patch`, `trace`, `connect` are
   implemented but appear nowhere in the README.
4. **Undocumented flags:** `--multi`, `--Redirect`, `-s/--secure`, `-f/--form`
   are used in examples without ever being listed.
5. **`--Redirect` is capitalized** (`cmd/root.go:79`), inconsistent with every
   other lowercase long flag.
6. **Line 179** — `## This allowed for efficient CLI development...` is a
   sentence marked up as an H2 heading; same at line 193. Reads as a formatting
   accident.
7. **No `--help` output sample**, no exit-code documentation.
8. **`CODE_OF_CONDUCT.md` contact** should be checked — it needs a real reporting
   address to be meaningful.

---

## Prioritized action plan

**Do first (correctness + security, all small diffs):**

1. Make TLS verification the default; add `-k/--insecure` opt-out — §1.1
2. Fix `trace` sending CONNECT — §3.1 (one word)
3. Add `Args: cobra.MinimumNArgs(1)` to `head`/`options`/`put`/`patch` — §2.1
4. Guard the five unchecked string indexes — §2.3, §2.4
5. Fix the `?a=1?b=2` query concatenation — §3.2
6. Add a client `Timeout` + `--timeout` flag — §5.1

**Do next (the root-cause refactors):**

7. Replace all 13 library `log.Fatal` calls with returned errors; fix `println(err)` — §4
8. Collapse `cmd/*.go` into a single factory — §7.3
9. Use `net/http` method constants instead of case-inconsistent literals — §3.4
10. Add a test suite — start with `formats` and `requests` URL/query building, using `httptest.Server` for `Send` — §6

**Then (hygiene):**

11. `go mod tidy`, bump Go and all deps, drop `ioutil` — §8
12. Delete the unused request subtypes and `basic_input.go` — §7.1, §7.2
13. Add CI, Makefile, golangci-lint, Dockerfile — §6
14. Reconcile the README with the actual command surface — §9

---

## What's already good

Worth stating plainly, since the list above is long:

- Clean package boundaries — `cmd` → `command` → `requests` → `response` is easy to follow.
- Transparent gzip/deflate/brotli decoding is more than most hobby HTTP clients bother with.
- Content-type-aware colorized output (JSON via `jsoncolor`, HTML via `chroma`) is a real usability win.
- The multipart file-upload design — `@!` prefix with `~`/`/`/relative path resolution — is a genuinely thoughtful bit of UX.
- `go build` and `go vet` both pass with zero findings on a first checkout.
