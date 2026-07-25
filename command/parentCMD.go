// Package command turns parsed CLI flags into a request and prints the response.
package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/formats"
	rq "github.com/mohamadkrayem/requestCLI/requests"
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

	Form      bool
	Multipart bool
	Redirect  bool
}

// Run builds and sends the request described by opts, then prints the response.
func Run(method string, args []string, opts *Options) error {
	if len(args) == 0 {
		return errors.New("a URL is required")
	}

	url, err := rq.GenerateUrl(args[0], opts.HTTP, opts.QueryParams)
	if err != nil {
		return err
	}

	request := rq.NewRequest(method, url)

	for key, value := range opts.Cookies {
		request.WithCookie(key, value)
	}

	if basicAuth := auth.NewBaseAuthFromMap(opts.Auth); basicAuth.Username != "" {
		request.BasicAuth = basicAuth
	}

	// Both header sources merge, so -n and --headers can be combined.
	if len(opts.HeadersJS) > 0 {
		request.WithHeadersMap(opts.HeadersJS)
	}
	if opts.Headersjs != "" {
		if err := request.WithHeaders(opts.Headersjs); err != nil {
			return err
		}
	}

	if opts.BodyJS != "" {
		if err := request.WithBody(opts.BodyJS, opts.Form, opts.Multipart); err != nil {
			return err
		}
	}

	resp, err := request.Send(rq.SendOptions{
		ShowStatus:  opts.ShowStatus,
		ShowHeaders: opts.ShowHeaders,
		ShowBody:    opts.ShowBody,
		Redirect:    opts.Redirect,
		Insecure:    opts.Insecure,
		Timeout:     opts.Timeout,
	})
	if err != nil {
		return err
	}

	resp.PrintResponse()
	return nil
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
