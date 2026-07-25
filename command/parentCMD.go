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

// maxInputSize caps a single stdin-supplied JSON document.
const maxInputSize = 1 << 20 // 1 MiB

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

	HTTP     bool
	Insecure bool
	Timeout  time.Duration

	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	Style       string

	Form      bool
	Multipart bool
	Redirect  bool
}

// Run parses args[0] as the URL and args[1:] as request items, builds and
// sends the request described by opts and the items, then prints the
// response.
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

	url, err := core.GenerateUrl(args[0], opts.HTTP, opts.QueryParams)
	if err != nil {
		return err
	}

	request := core.NewRequest(method, url)

	for key, value := range opts.Cookies {
		request.WithCookie(key, value)
	}

	if basicAuth := auth.NewBaseAuthFromMap(opts.Auth); basicAuth.Username != "" {
		request.BasicAuth = basicAuth
	}

	// Headers: -n/--Nheaders -> --headers (stdin JSON) -> request items,
	// later wins. Items are applied last because they are the most explicit
	// source, and a Key: item must be able to unset what an earlier source set.
	if len(opts.HeadersJS) > 0 {
		request.WithHeadersMap(opts.HeadersJS)
	}
	if opts.Headersjs != "" {
		if err := request.WithHeaders(opts.Headersjs); err != nil {
			return err
		}
	}
	applyHeaderItems(&request, items)

	// Body: -b/--Nbody, --body and body-carrying request items are all
	// explicit sources, and at most one of them may supply a body. Merging a
	// hand-written JSON body with item-generated fields would require
	// decoding and re-encoding it, reintroducing exactly the key reordering
	// the M1.5 decision eliminated, so the two are mutually exclusive instead.
	switch {
	case items.HasBody() && (opts.Body || opts.BodyJS != ""):
		return errors.New("request items and --Nbody/--body both provide a body; use one or the other")

	case items.HasBody():
		if err := applyBodyItems(&request, items, opts); err != nil {
			return err
		}

	case opts.BodyJS != "":
		if err := request.WithBody(opts.BodyJS, opts.Form, opts.Multipart); err != nil {
			return err
		}
	}

	// Query: -q was already applied by GenerateUrl above. An item overrides
	// -q for the same key; repeated items for one key all survive.
	if err := request.MergeQueryValues(items.QueryValues()); err != nil {
		return err
	}

	result, err := request.Send(core.SendOptions{
		Redirect: opts.Redirect,
		Insecure: opts.Insecure,
		Timeout:  opts.Timeout,
	})
	if err != nil {
		return err
	}

	// Colour is resolved here, once, and passed down. No renderer decides for
	// itself whether it is talking to a terminal.
	fmt.Println(render.Render(result, render.Options{
		ShowStatus:  opts.ShowStatus,
		ShowHeaders: opts.ShowHeaders,
		ShowBody:    opts.ShowBody,
		Color:       render.ColorEnabled(os.Stdout),
		Style:       opts.Style,
	}))
	return nil
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
			switch decoded.(type) {
			case []any, map[string]any:
				// Written verbatim, as the user typed it, so it stays compact
				// JSON text rather than Go's "%v" formatting of a slice/map.
				values[item.Key] = item.Value
			default:
				values[item.Key] = decoded
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

// PrepareInput reads nested JSON from stdin when --body or --headers were given.
//
// When both are set, headers are read first and the body second, matching the
// order documented in the README.
func PrepareInput(opts *Options) error {
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

	return nil
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
