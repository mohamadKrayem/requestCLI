# rq

`rq`, inspired by tools like HTTPie and curl, is designed to be beginner-friendly
with strong JSON operations, making it easy to execute various HTTP requests.
The app offers advanced features such as request items, piped stdin bodies,
handling cookies, basic authentication, multipart file uploads, and formatted,
colorized output.

> **The `requestCLI` binary name is gone as of v1.3.0.** It shipped as a
> deprecated alias throughout v1.2.0; the command is now `rq` only. If a script
> still calls `requestCLI`, point it at `rq` — nothing else about the invocation
> changed. The Go module path is unchanged.

## Installation

Make sure to have Go installed on your system. You can download it from [https://go.dev/doc/install](https://go.dev/doc/install). Verify the installation by running the following command in your terminal:

```shell
$ go version
```

To install `rq`:

```shell
$ go install github.com/mohamadkrayem/requestCLI/cmd/rq@latest
```

> The module root is no longer an installable path. `go install` names a binary
> after the last element of its import path, so installing the root could only
> ever produce `requestCLI`; `@latest` from that path now fails rather than
> quietly installing the old name. Use the `cmd/rq` path above.

Prebuilt binaries for Linux, macOS and Windows (amd64 and arm64) are attached to
each [release](https://github.com/mohamadKrayem/requestCLI/releases).

Or build from a checkout:

```shell
$ git clone https://github.com/mohamadkrayem/requestCLI.git
$ cd requestCLI
$ make build
```

`make build` produces `rq`.

### Docker

```shell
$ docker build -t rq .
$ docker run --rm rq get example.com -S
```

The image is a small Alpine layer holding a statically linked binary, runs as an
unprivileged user (uid 10001), and ships `ca-certificates` because TLS
verification is on by default. `rq` is the entrypoint, so arguments go straight
after the image name.

Two flags matter more than usual in a container:

```shell
# -i keeps stdin attached, which piped request bodies need.
$ echo '{"name":"ada"}' | docker run --rm -i rq post example.com

# Reaching a service on the host, not inside the container.
$ docker run --rm --network host rq get 127.0.0.1:8080/health --http
```

`make docker-build` tags the image with `git describe`, and stamps that version
into the binary so `rq --version` and the `User-Agent` header report it:

```shell
$ make docker-build              # rq:<version> and rq:latest
$ make docker-run ARGS="get example.com -S"
```

For local development, `docker-compose.yml` runs the fixture server used by the
test suite (see [TESTING.md](TESTING.md)) so you can try requests without
hitting a real API:

```shell
$ make docker-up                                       # fixture on :8080/:8443
$ docker compose run --rm rq get echoserver:8080/json --http -B
$ make docker-down
```

`make docker-smoke` runs an end-to-end check of the image itself — non-root
user, trust store, stdin, and real requests against that fixture.

## Usage

The primary way to describe a request is with a URL followed by **request
items** — HTTPie-style `key:value` / `key=value` positional arguments that
build headers, query parameters and the body without any JSON quoting:

```shell
$ rq post example.com/login username=Mohamad password=secret X-Api-Token:abc123
```

That one line sends a POST with `{"username":"Mohamad","password":"secret"}` as
a JSON body and an `X-Api-Token: abc123` header — no `-b`, no escaped quotes.

For more information:

```shell
$ rq --help
$ rq get --help
```

### Commands

| Command   | Method  | Aliases  |
| --------- | ------- | -------- |
| `get`     | GET     |          |
| `post`    | POST    |          |
| `put`     | PUT     |          |
| `patch`   | PATCH   |          |
| `del`     | DELETE  | `delete` |
| `head`    | HEAD    |          |
| `options` | OPTIONS |          |
| `trace`   | TRACE   |          |
| `connect` | CONNECT | `conn`   |

Every command takes a URL, followed by zero or more request items:

```shell
$ rq <command> URL [REQUEST_ITEM ...]
```

Request items must come **after** the URL.

### Request items

| Item         | Kind                  | Effect |
| ------------ | --------------------- | ------ |
| `key=value`  | Field                 | JSON string field (or form field with `-f`) |
| `key:=value` | RawField              | Verbatim JSON value — `age:=22` sends the number `22`, not the string `"22"` |
| `Key:value`  | Header                | Sets a header |
| `Key:`       | HeaderUnset           | Removes a header, including one `rq` would otherwise send by default |
| `key==value` | Query                 | Adds a query parameter |
| `key=@path`  | FileField             | Field value read from a file, sent as text |
| `key@path`   | FileUpload            | Multipart file attachment |

```shell
$ rq post example.com name=Mohamad age:=22          # JSON body: {"name":"Mohamad","age":22}
$ rq get example.com page==2 per_page==10           # ?page=2&per_page=10
$ rq get example.com X-Api-Token:abc123             # header
$ rq get example.com User-Agent:                    # unset the default User-Agent
$ rq post example.com/upload avatar@./me.png        # multipart file upload
$ rq post example.com bio=@./bio.txt                # field value read from a file
```

Rules that matter in practice:

- The first separator found in an argument wins, scanned left to right, two-character
  forms (`==`, `:=`, `=@`) checked before one-character ones. This is what lets
  `email=a@b.com` work as a plain field (the `=` comes first) while
  `avatar@./me.png` is a file upload.
- A key may escape a literal separator character with `\`: `foo\:bar=1` sends a
  field named `foo:bar`. Values are always taken verbatim — no escaping inside a
  value, which is what shell quoting is for.
- Repeated keys: for headers and query, later occurrences all survive in the
  order given (`tag==one tag==two` sends both). For body fields, the **last**
  occurrence wins.
- A request item and `-b`/`--Nbody`/`--body` cannot both supply a body — pick
  one. Merging a hand-typed JSON body with generated fields would require
  decoding it, which reorders keys and rounds large integers.
- On a verb that carries data in the query (`GET`, `DELETE`, `HEAD`, `TRACE`,
  `OPTIONS`, `CONNECT`), body-style items become query parameters instead,
  matching the existing behavior of `-b` on `GET`. A file upload on one of
  these verbs is an error — use `POST`, `PUT` or `PATCH`.
- A `key@file` item implies multipart automatically. Combining it with `-f` is
  an error (a file cannot be sent as a url-encoded form).

Everything below — `-n`, `-q`, `-b`, `--headers`, `--body`, `-f`, `--multi` — is
still fully supported and combines with request items; see
[Flags](#flags) and [Precedence](#precedence-flags-and-request-items) below.

### Flags

| Flag             | Short | Description                                                  |
| ---------------- | ----- | -------------------------------------------------------------- |
| `--Nbody`        | `-b`  | Body as JSON on a single line.                                  |
| `--body`         |       | Read a multi-line JSON body from stdin, terminated by `;`.      |
| `--Nheaders`     | `-n`  | Headers as `key=value` pairs.                                   |
| `--headers`      |       | Read multi-line JSON headers from stdin, terminated by `;`.     |
| `--query`        | `-q`  | Query parameters as `key=value` pairs.                          |
| `--cookie`       | `-c`  | Cookies as `key=value` pairs.                                   |
| `--auth`         | `-a`  | Basic auth, e.g. `username=me,password=secret`.                 |
| `--form`         | `-f`  | Send a url-encoded form.                                        |
| `--multi`        |       | Send a multipart form (supports file uploads).                  |
| `--printB`       | `-B`  | Print the response body.                                         |
| `--printH`       | `-H`  | Print the response headers.                                     |
| `--printS`       | `-S`  | Print the response status line.                                  |
| `--verbose`      | `-v`  | Also print the request that was sent.                            |
| `--check-status` |       | Exit with HTTPie's 3/4/5 status codes on a 3xx/4xx/5xx response. |
| `--ignore-stdin` |       | Never read a request body from piped stdin.                      |
| `--redirect`     |       | Follow redirects.                                                |
| `--http`         |       | Force plain HTTP instead of HTTPS.                                |
| `--insecure`     | `-k`  | Skip TLS certificate verification (dangerous).                    |
| `--timeout`      |       | Overall request timeout (default `30s`).                         |
| `--style`        |       | Syntax highlighting theme (default `monokai`); tab-completes.     |

The `-B`, `-H` and `-S` flags combine: `-H -B` prints headers and body. Passing
none of them prints everything. `-v` is independent of that set: `-v` alone
shows the request plus the full response; `-v -B` shows the request plus the
response body only.

### Precedence: flags and request items

Headers, query and body can each come from more than one source. The rule is
always "later/more explicit wins":

- **Headers**: `-n`/`--Nheaders`, then `--headers` (stdin JSON), then request
  items — items are applied last, so `Key:` can unset a header any earlier
  source set.
- **Query**: `-q` is applied first. Then, for each distinct key carried by a
  `key==value` item, any existing `-q` value for that key is replaced;
  repeated items for the same key all survive.
- **Body**: `-b`/`--Nbody`, `--body`, and body-carrying request items are all
  explicit and mutually exclusive with each other (pick one). Piped stdin is
  implicit and is only used when none of the explicit sources are present.

### Piped stdin body

If nothing else supplies a body — no `-b`, no `--body`, no `--headers`, no
`--ignore-stdin`, and no body-carrying request item — and stdin is a pipe
rather than a terminal, `rq` reads all of stdin (capped at 10 MiB) and sends it
as the body:

```shell
$ echo '{"name":"Mohamad"}' | rq post example.com -B
```

An empty pipe (`< /dev/null`, a closed-stdin CI runner) produces no body and is
not an error. Pass `--ignore-stdin` if your shell leaves an open pipe on stdin
that you do not want read as a body.

### `-v` / `--verbose`

`-v` prints the request that was actually sent — method, request line, final
headers (including defaults, auth and cookies) and body — followed by a blank
line and then the response, on the same stream:

```shell
$ rq get localhost:8080/json -v -S
GET /json HTTP/1.1
Host:   localhost:8080
Accept:   */*
Accept-Encoding:   gzip, deflate, br
User-Agent:   rq/1.2.0

HTTP/1.1 200 OK
```

`Authorization` and `Cookie` are shown unmasked.

### `.http` files

Requests can live in a file instead of a command line. The format is the one VS
Code REST Client, the JetBrains HTTP Client and kulala.nvim already read, so
files written for any of them work unchanged — and files written here stay
readable in all of them.

```http
@base = https://api.example.com
@who = ada

### Create a user
POST {{base}}/users
Content-Type: application/json
X-Token: {{token}}

{"name":"{{who}}"}

### List users
GET {{base}}/users
```

```shell
$ rq run api.http                        # every request, in order
$ rq run api.http --name "List users"    # just that one
$ rq run api.http --var token=abc123     # supply a {{variable}}
```

Every global flag applies, so `-B`, `-v`, `--check-status` and the rest behave
exactly as they do on the command line.

| Syntax | Meaning |
| ------ | ------- |
| `###` | Separates requests; trailing text names the one that follows |
| `# @name x` | Names a request explicitly |
| `@name = value` | A file-level variable |
| `{{name}}` | Substituted from `@name` or `--var` (`--var` wins) |
| `# …` / `// …` | Comment |
| `< ./body.json` | Body read from a file, relative to the `.http` file |
| `GET url HTTP/1.1` | The version is accepted and ignored |
| a bare URL | Treated as `GET` |

Requests run in order and **stop at the first failure**, because a file is
usually a sequence and continuing past a broken step buries the real error
under a cascade. When more than one request runs, a heading naming each goes to
stderr, so a piped run carries response bodies and nothing else.

Errors report `file:line:`, which editors and terminals turn into a jump:

```
$ rq run api.http
Error: api.http:7: unknown variable {{token}}
```

### Collections

A collection is a **directory** containing an `rq.toml`, found by walking up
from the request file the way git finds `.git`. There is no manifest, no export
step and no ids: adding a request means creating a file, and renaming one is
`mv`.

```
api/
├── rq.toml                  # defaults inherited by everything below
├── environments/
│   ├── dev.toml
│   └── staging.toml
├── .rq.secrets.toml         # gitignored, never committed
└── users/
    ├── rq.toml              # merges over the parent
    └── list.http
```

```toml
# api/rq.toml
[defaults]
base_url = "https://api.example.com"

[defaults.headers]
Accept = "application/json"

[defaults.auth]
type  = "bearer"
token = "{{secret:api_token}}"
```

Set the base URL and auth once; write requests as `GET {{base_url}}/users`.
Headers merge down the chain and a nested value wins. Auth is replaced
wholesale, so a subdirectory switching from bearer to basic does not inherit
half of what it replaced.

### Variables

Where a value comes from is part of the reference, so `{{token}}` is never
ambiguous the way it is in a GUI client:

| Reference | Resolves from |
| --- | --- |
| `{{base_url}}` | An ordinary variable, by the precedence below |
| `{{secret:api_token}}` | `.rq.secrets.toml`, then the environment — never committed |
| `{{env:HOME}}` | The process environment |
| `{{$uuid}}`, `{{$timestamp}}`, `{{$randomInt}}` | Generated once per request |

**Namespaces are not precedence levels.** A `{{secret:token}}` can never
silently shadow an ordinary `{{token}}` — the failure mode that makes
credential bugs hard to see.

Ordinary variables resolve lowest to highest:

| # | Scope | Lives in |
| - | ----- | -------- |
| 1 | Built-in | `{{$uuid}}` and friends |
| 2 | System-wide | `~/.config/rq/vars.toml` — machine-local, warns when used |
| 3 | Collection | `rq.toml` at the collection root |
| 4 | Nested collection | `rq.toml` in a subdirectory |
| 5 | Environment | `environments/<name>.toml`, via `--env` |
| 6 | File | `@name = value` at the top of a `.http` file |
| 7 | Request | `# @var name = value` above a request |
| 8 | Command line | `--var name=value` |

Resolving from the machine-local scope prints a warning: it works for you and
fails for a teammate, which is a confusing thing to debug remotely.

### `rq vars`

```shell
$ rq vars users/list.http --env staging
$uuid            = (generated per request)  (built-in)
api_ver          = staging-v2               (api/environments/staging.toml)
base_url         = https://api.example.com  (api/rq.toml)
page             = 7                        (api/users/list.http:3)
secret:api_token = ****                     (api/.rq.secrets.toml)
```

Modelled on `git config --list --show-origin`. It reports variables referenced
by inherited headers and auth as well as by the file, because those are what
authenticate the request. Secrets are masked here and in `-v` output; pass
`--show-secrets` to see them.

> **Not yet implemented:** the OS keychain as a secret source, and
> `{{login.response.body.token}}` references to an earlier request. A file
> using either reports the variable as unknown rather than silently sending the
> wrong thing.

### Streaming and server-sent events

A `text/event-stream` response is rendered incrementally, frame by frame, with
no flag:

```shell
$ rq get api.example.com/events
● delta  {"index":0,"text":"chunk 0"}
● delta  {"index":1,"text":"chunk 1"}
● delta  {"index":2,"text":"chunk 2"}
3 events · 84 B · first 340ms · total 2.90s · 1.0 events/s
```

Events go to **stdout**, the closing summary to **stderr**, so a pipe carries
events and nothing else.

| Flag | Effect |
| ---- | ------ |
| *(none)* | Automatic on `text/event-stream` |
| `--stream` | Force it for a server that streams under another content type |
| `--raw` | Print each frame as it arrived, comment lines included |
| `-v` | Add each event's offset from the start of the request |

`--raw` is for debugging the framing rather than the payload:

```shell
$ rq get api.example.com/events --raw
: keepalive
event: delta
data: {"index":0,"text":"chunk 0"}
```

Keepalive comments are framing, not data. They are not rendered and not
counted, but `--raw` still shows them.

**`--timeout` means something different for a stream.** It bounds connecting
and the response headers only — not the life of the stream, which would
otherwise be cut off after 30s by default. A buffered response is still bounded
end to end.

Press Ctrl-C to stop. Whatever arrived is kept, the summary is still printed,
and the exit code is 130.

### Exit codes

| Code | Meaning |
| ---- | ------- |
| 0    | Response received. Without `--check-status`, any status counts. With it, status < 300, or a 3xx that was followed with `--redirect`. |
| 1    | Usage or local error: bad flag, unparseable request item, invalid URL, invalid JSON, unreadable file. |
| 2    | Transport failure: DNS, connection refused, TLS, timeout. |
| 3    | `--check-status` and a 3xx that was **not** followed. |
| 4    | `--check-status` and a 4xx. |
| 5    | `--check-status` and a 5xx. |

Without `--check-status`, an HTTP error status still exits 0 — the request
itself succeeded. 3/4/5 are HTTPie's codes verbatim; 2 is new, so a script can
tell "could not reach the server" apart from "server said 404".

```shell
$ rq get example.com/missing --check-status; echo $?
4
```

### Shell completions

Cobra's built-in `completion` command generates a script for your shell:

```shell
$ rq completion bash > /etc/bash_completion.d/rq
$ rq completion zsh  > "${fpath[1]}/_rq"
$ rq completion fish > ~/.config/fish/completions/rq.fish
```

Method commands (`get`, `post`, ...) do not offer filenames for their
positional arguments, since those are always a URL or a request item.
`--style` tab-completes the available theme names.

### Output

Response bodies are shown exactly as the server sent them. JSON is pretty-printed
straight from the raw bytes, so large integers keep their precision, keys keep
their original order, and duplicate keys are not collapsed — the display never
decodes the payload.

JSON, HTML, XML, YAML, JavaScript, CSS, TOML, SQL, Markdown and GraphQL are
syntax-highlighted. Pass `--style` to change the theme.

Colour is enabled only when stdout is a terminal, and is disabled entirely if
`NO_COLOR` is set or `TERM=dumb`, so piped output is always clean:

```shell
$ rq get example.com -B | grep name   # no escape sequences
$ NO_COLOR=1 rq get example.com -B
```

Binary responses are described rather than dumped:

```shell
$ rq get example.com/logo.png -B
[binary data: 12.4 kB, image/png — not shown]
```

### URL scheme

Scheme-less URLs default to **https**, except loopback hosts (`localhost`,
`127.0.0.1`, `[::1]`) which default to **http** so local development keeps
working. Pass `--http` to force plain HTTP for any host.

```shell
$ rq get example.com          # -> https://example.com
$ rq get localhost:3000       # -> http://localhost:3000
$ rq get example.com --http   # -> http://example.com
```

### TLS

Certificate verification is **on** by default. Use `-k` / `--insecure` to talk
to a server with a self-signed certificate — this removes protection against
man-in-the-middle attacks, so use it only against hosts you control.

```shell
$ rq get https://self-signed.local -k
```

## Examples

Request items, the primary way to build a request:

```shell
$ rq post example.com/login username=Mohamad password=secret
$ rq get example.com q==search per_page==10
$ rq post example.com X-Api-Token:abc123 name=Mohamad
$ rq post example.com/upload avatar@./photo.png caption=vacation
```

Simple headers and a JSON body, using flags:

```shell
$ rq post example.com -n X-API-Token=123 -b='{"name":"Mohamad"}'
```

Multiple simple headers:

```shell
$ rq post example.com -n X-API-Token-1=123 -n X-API-Token-2=456
```

Or separated by a comma:

```shell
$ rq post example.com -n X-API-Token-1=123,X-API-Token-2=456
```

---

Complex headers and body — use `--headers` and `--body`, and terminate each
block with `;`:

```shell
$ rq put example.com --headers --body
{
	"X-API-Token": 123
};
{
	"name":"Mohamad",
	"arrayOfNbs": [1, 2, 3],
	"nestedJS": { "w":"2" }
};
```

Write complete JSON, including the closing brace or bracket. Top-level arrays
are supported, and blank lines are ignored.

---

Cookies:

```shell
$ rq get example.com -n X-API-Token=123 -c key=value
```

Basic authentication:

```shell
$ rq get example.com --auth username=Mohamad,password=pass123
```

> **Note**: credentials passed this way land in your shell history and are
> visible in `ps`. Prefer a throwaway credential for anything sensitive.

Query string parameters:

```shell
$ rq get example.com -q q=queryExample,per_page=1
```

Output selection:

```shell
$ rq get example.com -H        # headers only
$ rq get example.com -H -B     # headers and body
```

Other methods:

```shell
$ rq del example.com
$ rq post "http://example.com" -b='{"name":"Example"}'
$ rq put example.com -b='{"name":"Example"}'
```

---

## Sending forms and files

Multipart form with files, via request items:

```shell
$ rq post example.com/form avatar@~/photo.jpeg name=Mohamad age:=22
```

Or with `--multi --body`:

```shell
$ rq post example.com/form --multi --body
{
	"@!image":"~/justForTesting/OIG.jpeg",
	"@!resume":"~/justForTesting/Mohamad_Krayem_2023_CV.docx",
	"@!letter":"~/justForTesting/letter.txt",
	"name":"Mohamad",
	"age":22
};
```

File fields in `--multi --body` JSON must begin with `@!`; normal data fields
carry no prefix. Path handling (shared with request-item file paths):

- `~` — resolved against your home directory.
- `/` — used as-is.
- anything else — resolved against the current working directory.

---

Url-encoded form:

```shell
$ rq post example.com -f --body
{
	"key1":"value",
	"key2":21
};
```

> **Note**: use `-b` for a single-line body or `--body` for a multi-line one.

---

## Development

```shell
$ make test     # go test -race ./...
$ make cover    # coverage summary
$ make vet
$ make lint     # requires golangci-lint
$ make build
```

## Testing

See [TESTING.md](TESTING.md) for the full guide, including manual scenarios.

```shell
$ make test     # unit and integration tests, no network needed
$ make smoke    # end-to-end run of the real binary against a local fixture
$ make server   # start the fixture server for manual testing
```

## Releasing

Pushing a `v*` tag runs the [Release workflow](.github/workflows/release.yml):
it checks the tag against `core.Version`, runs vet, tests and the smoke suite,
builds all six platform archives, and opens a **draft** release with them
attached. Write the upgrade notes there, then publish.

```shell
$ make release              # rehearse locally; writes dist/
$ VERSION=1.3.0 make release
```

Before tagging, bump `core.Version` in `core/request.go` to match. The workflow
refuses a tag that disagrees with it, because `go install` applies no link
flags — a module-path install reports whatever the source says.

The archives are byte-reproducible: same tag, same toolchain, same checksums.
To audit a published release rather than trust it, check the tag out, run
`make release`, and compare `dist/SHA256SUMS` against the one attached to it.

## Technologies Used

- Golang
- Cobra
- chroma (syntax highlighting)
- tidwall/pretty (lossless JSON formatting)

## Future Features

- JWT authentication
- Digest authentication
- Proxies
- Client SSL certificates
- Downloading response bodies to a file
- Secret masking in `-v` output (`{{secret:}}` namespace)

## License

See [LICENSE](LICENSE).
