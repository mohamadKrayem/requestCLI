// Package command turns parsed CLI flags into a request and prints the response.
package command

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/core"
	"github.com/mohamadkrayem/requestCLI/formats"
	"github.com/mohamadkrayem/requestCLI/input"
	"github.com/mohamadkrayem/requestCLI/render"
	"github.com/mohamadkrayem/requestCLI/reqitem"
)

// maxInputSize caps a single stdin-supplied JSON document (--headers/--body).
const maxInputSize = 1 << 20 // 1 MiB

// maxStdinBodySize caps a piped stdin body (the implicit source), distinct
// from maxInputSize above.
const maxStdinBodySize = 10 << 20 // 10 MiB

// Options holds every flag value for one invocation.
type Options struct {
	QueryParams map[string]string
	Cookies     map[string]string
	Auth        map[string]string

	Body      bool
	BodyJS    string
	Headers   bool
	HeadersJS map[string]string
	Headersjs formats.Json

	// IgnoreStdin suppresses the implicit read of a piped body. It is the
	// escape hatch for a shell that leaves an open pipe on stdin.
	IgnoreStdin bool
	// StdinBody holds a body read from a piped stdin. It is populated by
	// PrepareInput, distinct from BodyJS which only ever comes from an
	// explicit -b/--Nbody or --body source.
	StdinBody string

	HTTP     bool
	Insecure bool
	Timeout  time.Duration

	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	Style       string

	// Verbose shows the request that was sent (-v), in addition to whatever
	// the Show flags already select from the response.
	Verbose bool
	// CheckStatus turns an HTTP error status into a non-zero exit code via
	// ExitError, using HTTPie's 3/4/5 scheme. It never changes what is
	// rendered — only the exit code.
	CheckStatus bool

	Form      bool
	Multipart bool
	Redirect  bool
}

// ExitError carries a specific process exit code. Err is nil when the failure
// is only an HTTP status, in which case nothing extra is printed to stderr —
// the rendered status line has already said it.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("request completed with exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// Run parses args[0] as the URL and args[1:] as request items, assembles the
// request, sends it and prints the result.
//
// The order below is fixed and must not be reordered: parse items ->
// PrepareInput -> buildRequest -> applyItems -> Send -> render -> checkStatus.
// Rendering always happens before checkStatus runs, so --check-status only
// ever changes the exit code, never the output.
func Run(method string, args []string, opts *Options) error {
	if len(args) == 0 {
		return errors.New("a URL is required")
	}

	// The URL is never item-parsed: args[1:] only, so a URL containing "="
	// or ":" (a query string, a scheme-less host:port) is structurally safe.
	items, err := reqitem.Parse(args[1:])
	if err != nil {
		return err
	}

	if err := PrepareInput(opts, items); err != nil {
		return err
	}

	request, err := buildRequest(method, args[0], items, opts)
	if err != nil {
		return err
	}

	if err := applyItems(request, items, opts); err != nil {
		return err
	}

	result, err := request.Send(core.SendOptions{
		Redirect: opts.Redirect,
		Insecure: opts.Insecure,
		Timeout:  opts.Timeout,
	})
	if err != nil {
		// A transport failure (DNS, connect, TLS, timeout) gets its own exit
		// code, distinct from a request that could not be built at all, so a
		// script can tell "could not reach the server" from a usage error.
		var transportErr *core.TransportError
		if errors.As(err, &transportErr) {
			return &ExitError{Code: 2, Err: err}
		}
		return err
	}

	// Colour is resolved here, once, and passed down. No renderer decides for
	// itself whether it is talking to a terminal.
	fmt.Println(render.Render(result, render.Options{
		ShowStatus:  opts.ShowStatus,
		ShowHeaders: opts.ShowHeaders,
		ShowBody:    opts.ShowBody,
		ShowRequest: opts.Verbose,
		Color:       render.ColorEnabled(os.Stdout),
		Style:       opts.Style,
	}))

	return checkStatus(result, opts)
}

// buildRequest assembles the request up to (but not including) the items and
// the body: the URL, cookies, basic auth and the flag-sourced headers
// (-n/--Nheaders, --headers).
func buildRequest(method, rawURL string, items reqitem.Items, opts *Options) (*core.BaseRequest, error) {
	url, err := core.GenerateUrl(rawURL, opts.HTTP, opts.QueryParams)
	if err != nil {
		return nil, err
	}

	request := core.NewRequest(method, url)

	for key, value := range opts.Cookies {
		request.WithCookie(key, value)
	}

	if basicAuth := auth.NewBaseAuthFromMap(opts.Auth); basicAuth.Username != "" {
		request.BasicAuth = basicAuth
	}

	// Headers: -n/--Nheaders -> --headers (stdin JSON). Request items are
	// applied afterward, in applyItems, because they are the most explicit
	// source and a Key: item must be able to unset what an earlier source set.
	if len(opts.HeadersJS) > 0 {
		request.WithHeadersMap(opts.HeadersJS)
	}
	if opts.Headersjs != "" {
		if err := request.WithHeaders(opts.Headersjs); err != nil {
			return nil, err
		}
	}

	return &request, nil
}

// applyItems layers the request items, and the body (explicit or piped
// stdin), on top of what buildRequest already assembled.
func applyItems(request *core.BaseRequest, items reqitem.Items, opts *Options) error {
	applyHeaderItems(request, items)

	// Body: -b/--Nbody, --body and body-carrying request items are all
	// explicit sources, and at most one of them may supply a body. Merging a
	// hand-written JSON body with item-generated fields would require
	// decoding and re-encoding it, reintroducing exactly the key reordering
	// the M1.5 decision eliminated, so the two are mutually exclusive instead.
	// Piped stdin is implicit and is only ever reached when none of the
	// explicit sources are present — PrepareInput already enforced that.
	switch {
	case items.HasBody() && (opts.Body || opts.BodyJS != ""):
		return errors.New("request items and --Nbody/--body both provide a body; use one or the other")

	case items.HasBody():
		if err := applyBodyItems(request, items, opts); err != nil {
			return err
		}

	case opts.BodyJS != "":
		if err := request.WithBody(opts.BodyJS, opts.Form, opts.Multipart); err != nil {
			return err
		}

	case opts.StdinBody != "":
		if err := request.WithBody(opts.StdinBody, opts.Form, opts.Multipart); err != nil {
			return err
		}
	}

	// Query: -q was already applied by GenerateUrl in buildRequest. An item
	// overrides -q for the same key; repeated items for one key all survive.
	return request.MergeQueryValues(items.QueryValues())
}

// checkStatus turns --check-status into HTTPie's 3/4/5 exit-code scheme, with
// 2 reserved for a transport failure (checked by the caller via
// errors.As(err, &*core.TransportError), since that never reaches here: Send
// already failed before there was a Result to check).
//
// Rendering has already happened by the time this runs; this only ever
// changes the exit code, never the output. Without --check-status, an HTTP
// error status still exits 0.
func checkStatus(r *core.Result, opts *Options) error {
	if !opts.CheckStatus {
		return nil
	}

	switch {
	case r.StatusCode < 300:
		return nil
	case r.StatusCode < 400:
		if opts.Redirect {
			// The redirect was followed, so the final status is what matters,
			// and it already passed the check above (or one of the ones below).
			return nil
		}
		return &ExitError{Code: 3}
	case r.StatusCode < 500:
		return &ExitError{Code: 4}
	default:
		return &ExitError{Code: 5}
	}
}

// applyHeaderItems applies the Header/HeaderUnset items on top of whatever
// headers -n and --headers already set.
//
// It walks the items in the order they were written rather than using
// HeaderOps, which groups sets and unsets into separate lists. Grouping would
// make an unset beat a set no matter which the user typed last, so
// `X-Token: X-Token:a` would drop the value instead of setting it. Across
// sources the order is unchanged: -n/--headers, then items, later wins.
func applyHeaderItems(request *core.BaseRequest, items reqitem.Items) {
	for _, item := range items {
		switch item.Kind {
		case reqitem.Header:
			request.WithHeader(item.Key, item.Value)
		case reqitem.HeaderUnset:
			request.WithoutHeader(item.Key)
		}
	}
}

// applyBodyItems attaches body-carrying request items to the request, picking
// the encoding named by opts.Form/opts.Multipart, the implied-multipart rule
// for a key@file item, or query-parameter routing for a verb that carries its
// data there (GET, DELETE, HEAD, TRACE, OPTIONS, CONNECT).
func applyBodyItems(request *core.BaseRequest, items reqitem.Items, opts *Options) error {
	if core.SendsBodyInQuery(request.Method) {
		if items.HasFileUpload() {
			return fmt.Errorf("%s cannot send a file upload; use POST, PUT or PATCH", request.Method)
		}
		queryBody, err := bodyItemsAsQuery(items)
		if err != nil {
			return err
		}
		return request.AddQueryString(queryBody)
	}

	if request.Headers == nil {
		request.Headers = make(map[string]any)
	}

	switch {
	case items.HasFileUpload() && opts.Form:
		return errors.New("a file upload cannot be sent as a url-encoded form; drop -f")

	case items.HasFileUpload() || opts.Multipart:
		fields, err := items.MultipartFields()
		if err != nil {
			return err
		}
		multipartInput, err := input.NewMultipartInputFromFields(fields)
		if err != nil {
			return err
		}
		request.MultipartBody = multipartInput.Body
		request.Writer = multipartInput.Writer
		request.Headers["Content-Type"] = multipartInput.Writer.FormDataContentType()
		return nil

	case opts.Form:
		values, err := items.FormValues()
		if err != nil {
			return err
		}
		request.Body = values.Encode()
		request.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		return nil

	default:
		body, err := items.JSONBody()
		if err != nil {
			return err
		}
		request.Body = string(body)
		request.Headers["Content-Type"] = "application/json"
		return nil
	}
}

// bodyItemsAsQuery converts body-carrying items into query parameters for a
// verb that carries its data there, mirroring the existing map-based handling
// of e.g. -b '{"a":1}' on GET. Building a map rather than writing bytes in
// order is safe here, unlike JSONBody: a query string has no meaningful key
// order of its own, since url.Values.Encode already sorts keys.
func bodyItemsAsQuery(items reqitem.Items) (map[string]any, error) {
	values := map[string]any{}
	for _, item := range items {
		switch item.Kind {
		case reqitem.Field:
			values[item.Key] = item.Value

		case reqitem.FileField:
			path, err := input.ResolvePath(item.Value)
			if err != nil {
				return nil, fmt.Errorf("reading %s for %q: %w", item.Value, item.Arg, err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading %s for %q: %w", item.Value, item.Arg, err)
			}
			values[item.Key] = string(content)

		case reqitem.RawField:
			if !json.Valid([]byte(item.Value)) {
				return nil, fmt.Errorf("request item %q is not valid json: %s", item.Arg, item.Value)
			}
			var decoded any
			if err := json.Unmarshal([]byte(item.Value), &decoded); err != nil {
				return nil, fmt.Errorf("request item %q is not valid json: %s", item.Arg, item.Value)
			}
			switch text := decoded.(type) {
			case string:
				// A raw JSON string goes on the wire unquoted.
				values[item.Key] = text
			case nil:
				values[item.Key] = nil
			default:
				// Numbers, booleans, arrays and objects all go on verbatim, as
				// the user typed them. json.Unmarshal turns every number into a
				// float64, so using the decoded value would round
				// id:=1234567890123456789 to ...800 — the same corruption
				// JSONBody writes bytes in order to avoid. It would be
				// incoherent for GET to mangle what POST preserves.
				values[item.Key] = item.Value
			}
		}
	}
	return values, nil
}

// stdinScanner is created once and shared across reads.
//
// A fresh bufio.Scanner per call would buffer past the first ';' terminator and
// swallow the second document, breaking `--headers --body` together.
var stdinScanner *bufio.Scanner

func sharedStdinScanner() *bufio.Scanner {
	if stdinScanner == nil {
		stdinScanner = newScanner(os.Stdin)
	}
	return stdinScanner
}

// PrepareInput reads any body or headers that come from stdin.
//
// items is needed because a request item that supplies a body suppresses the
// implicit read of piped stdin.
//
// When --headers and --body are both set, headers are read first and the body
// second, matching the order documented in the README. That shared scanner
// path owns stdin whenever either flag is set — the piped-body check below
// never runs in that case — which is what preserves the `--headers --body`
// regression fix.
func PrepareInput(opts *Options, items reqitem.Items) error {
	scanner := sharedStdinScanner()

	if opts.Headers {
		raw, err := scanRequest(scanner)
		if err != nil {
			return fmt.Errorf("reading --headers: %w", err)
		}
		headers, err := formats.NewJson(raw)
		if err != nil {
			return fmt.Errorf("reading --headers: %w", err)
		}
		opts.Headersjs = headers
	}

	if opts.Body && opts.BodyJS == "" {
		raw, err := scanRequest(scanner)
		if err != nil {
			return fmt.Errorf("reading --body: %w", err)
		}
		opts.BodyJS = raw
	}

	if shouldReadStdinBody(opts, items) {
		body, err := readStdinBody()
		if err != nil {
			return err
		}
		opts.StdinBody = body
	}

	return nil
}

// shouldReadStdinBody reports whether piped stdin should be read as an
// implicit request body. Every explicit source must be absent, --ignore-stdin
// must not have been passed, and stdin must not be a character device — a
// terminal left attached to stdin must never make the command hang waiting
// for a body that will never arrive.
func shouldReadStdinBody(opts *Options, items reqitem.Items) bool {
	if opts.Headers || opts.Body || opts.IgnoreStdin || opts.BodyJS != "" || items.HasBody() {
		return false
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		// Unable to stat stdin: treat it conservatively as a terminal rather
		// than risk blocking on a read that may never finish.
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// readStdinBody reads all of piped stdin as the request body, capped at
// 10 MiB. An empty read is not an error, so `< /dev/null` and closed-stdin CI
// runners behave exactly as they did before piped bodies existed.
func readStdinBody() (string, error) {
	body, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinBodySize+1))
	if err != nil {
		return "", fmt.Errorf("reading piped body: %w", err)
	}
	if len(body) > maxStdinBodySize {
		return "", errors.New("piped body exceeds the 10 MiB limit")
	}
	return string(body), nil
}

func newScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxInputSize)
	return scanner
}

// scanRequest reads one multi-line JSON document, terminated by a line ending in ';'.
//
// Blank lines are skipped and the document is returned exactly as written, so
// both objects and top-level arrays round-trip correctly. The scanner is passed
// in so successive documents can be read from the same stream.
func scanRequest(scanner *bufio.Scanner) (string, error) {
	var input strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ";") {
			input.WriteString(strings.TrimSuffix(line, ";"))
			break
		}
		input.WriteString(line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	text := strings.TrimSpace(input.String())
	if text == "" {
		return "", errors.New("no input given; end your json with ';'")
	}
	return text, nil
}
