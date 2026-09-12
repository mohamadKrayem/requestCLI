package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mohamadkrayem/requestCLI/core"
	"github.com/mohamadkrayem/requestCLI/httpfile"
	"github.com/mohamadkrayem/requestCLI/render"
)

// RunFile parses a .http file and sends the requests in it.
//
// Requests run in file order and stop at the first failure, because a file is
// usually a sequence — log in, then use the token — and continuing past a
// broken step produces a cascade of failures that hides the real one.
func RunFile(path string, opts *Options) error {
	file, err := httpfile.ParseFile(path)
	if err != nil {
		return err
	}

	requests, err := selectRequests(file, opts.RequestName)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return fmt.Errorf("%s contains no requests", path)
	}

	resolver, err := buildResolver(file, path, opts)
	if err != nil {
		return err
	}

	// Headings only earn their place when there is more than one response to
	// tell apart.
	showHeadings := len(requests) > 1

	for _, parsed := range requests {
		resolved, err := resolver.Resolve(parsed, file.Dir, file.Path)
		if err != nil {
			return err
		}

		if showHeadings {
			// On stderr, like the stream summary: piping the run must yield
			// response bodies and nothing else.
			fmt.Fprintf(os.Stderr, "\n### %s\n", describe(resolved))
		}

		if err := sendResolved(resolved, opts); err != nil {
			return err
		}
	}

	return nil
}

// buildResolver assembles the variable sources, lowest precedence first.
//
// Only two scopes exist so far. The eventual model adds a system-wide file,
// collection defaults, nested collections, environments and request-level
// definitions between these two; each is one more Source inserted at the right
// index, which is why precedence lives in the order of this slice rather than
// in branching.
func buildResolver(file *httpfile.File, path string, opts *Options) (httpfile.Resolver, error) {
	flagVars := make(map[string]string, len(opts.Vars))
	for _, arg := range opts.Vars {
		name, value, err := httpfile.ParseVarFlag(arg)
		if err != nil {
			return httpfile.Resolver{}, err
		}
		flagVars[name] = value
	}

	return httpfile.Resolver{Sources: []httpfile.Source{
		httpfile.FileVars(file, path),
		httpfile.MapSource{Values: flagVars, Label: "--var"},
	}}, nil
}

// selectRequests returns the requests to run: all of them, or the one --name
// picked.
func selectRequests(file *httpfile.File, name string) ([]httpfile.Request, error) {
	if name == "" {
		return file.Requests, nil
	}

	for _, request := range file.Requests {
		if request.Name == name {
			return []httpfile.Request{request}, nil
		}
	}

	// Listing what is available turns a typo into a one-step fix.
	var available []string
	for _, request := range file.Requests {
		if request.Name != "" {
			available = append(available, request.Name)
		}
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no request named %q; this file has no named requests", name)
	}
	return nil, fmt.Errorf("no request named %q; available: %s", name, strings.Join(available, ", "))
}

// describe labels a request for a heading: its name, or its method and URL
// when it has none.
func describe(request httpfile.Request) string {
	if request.Name != "" {
		return request.Name
	}
	return request.Method + " " + request.URL
}

// sendResolved turns one resolved request into a core request, sends it and
// renders the result, reusing the same rendering and exit-code rules the
// command-line path uses.
func sendResolved(request httpfile.Request, opts *Options) error {
	url, err := core.GenerateURL(request.URL, opts.HTTP, opts.QueryParams)
	if err != nil {
		return err
	}

	built := core.NewRequest(request.Method, url)

	for key, value := range opts.Cookies {
		built.WithCookie(key, value)
	}
	// Flag-supplied headers first, so a header written in the file wins: the
	// file is the more specific statement of intent for the request it
	// describes.
	if len(opts.HeadersJS) > 0 {
		built.WithHeadersMap(opts.HeadersJS)
	}
	for _, header := range request.Headers {
		built.WithHeader(header.Name, header.Value)
	}

	if len(request.Body) > 0 {
		built.Body = string(request.Body)
	}

	result, err := built.Send(core.SendOptions{
		Redirect: opts.Redirect,
		Insecure: opts.Insecure,
		Timeout:  opts.Timeout,
		Stream:   opts.Stream,
	})
	if err != nil {
		var transportErr *core.TransportError
		if errors.As(err, &transportErr) {
			return &ExitError{Code: 2, Err: err}
		}
		return err
	}
	defer func() { _ = result.Close() }()

	renderOpts := render.Options{
		ShowStatus:      opts.ShowStatus,
		ShowHeaders:     opts.ShowHeaders,
		ShowBody:        opts.ShowBody,
		ShowRequest:     opts.Verbose,
		Color:           render.ColorEnabled(os.Stdout),
		Style:           opts.Style,
		Raw:             opts.Raw,
		ShowEventTiming: opts.Verbose,
	}

	if result.Stream != nil {
		return streamResult(result, opts, renderOpts)
	}

	fmt.Println(render.Render(result, renderOpts))

	return checkStatus(result, opts)
}
