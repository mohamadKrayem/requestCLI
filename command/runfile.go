package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/collection"
	"github.com/mohamadkrayem/requestCLI/core"
	"github.com/mohamadkrayem/requestCLI/httpfile"
	"github.com/mohamadkrayem/requestCLI/render"
)

// systemVarsPath is the machine-local variable file. It is the one scope that
// is never committed, which is why resolving from it earns a warning.
func systemVarsPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "rq", "vars.toml")
	}
	return ""
}

// scopes is everything needed to resolve a file's variables, assembled once
// per run.
type scopes struct {
	resolver   httpfile.Resolver
	defaults   collection.Defaults
	collection *collection.Collection
}

// RunFile parses a .http file and sends the requests in it.
//
// Requests run in file order and stop at the first failure, because a file is
// usually a sequence — log in, then use the token — and continuing past a
// broken step produces a cascade that hides the real one.
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

	built, err := buildScopes(file, path, opts)
	if err != nil {
		return err
	}

	// Headings only earn their place when there is more than one response to
	// tell apart.
	showHeadings := len(requests) > 1

	for _, parsed := range requests {
		// Request-level vars sit above the file's own and below --var, so they
		// are layered per request rather than once for the file.
		resolver := built.resolver
		if len(parsed.Vars) > 0 {
			resolver = withRequestVars(resolver, parsed, path)
		}

		// One expansion pass covers the collection defaults and the request
		// together. Sharing it is what resolves {{secret:...}} inside an
		// inherited auth token, collects those secrets for masking, and makes
		// a {{$uuid}} in a default header match the one in the body.
		expander := resolver.NewExpander(file.Path)

		defaults, err := expandDefaults(expander, built.defaults, parsed.Line)
		if err != nil {
			return err
		}

		request, err := expander.ExpandRequest(parsed, file.Dir)
		if err != nil {
			return err
		}

		warnSystemScoped(expander.SystemScoped())

		if showHeadings {
			// On stderr, like the stream summary: piping the run must yield
			// response bodies and nothing else.
			fmt.Fprintf(os.Stderr, "\n### %s\n", describe(request))
		}

		resolved := httpfile.Resolved{Request: request, Secrets: expander.Secrets()}
		if err := sendResolved(resolved, defaults, opts); err != nil {
			return err
		}
	}

	return nil
}

// buildScopes assembles the variable sources in precedence order, lowest
// first, plus the namespaces that sit outside that order.
//
// The order of this slice *is* the precedence model. Adding a scope means
// inserting a Source at the right index and changing nothing else.
func buildScopes(file *httpfile.File, path string, opts *Options) (scopes, error) {
	var (
		sources []httpfile.Source
		found   *collection.Collection
	)

	// 2. System-wide. Machine-local and never committed.
	if systemPath := systemVarsPath(); systemPath != "" {
		values, err := loadFlatIfPresent(systemPath)
		if err != nil {
			return scopes{}, err
		}
		if len(values) > 0 {
			sources = append(sources, httpfile.MapSource{
				Values: values, Label: systemPath, System: true,
			})
		}
	}

	// 3 and 4. Collection defaults, root first so a nested rq.toml wins.
	discovered, err := collection.Discover(file.Dir)
	switch {
	case err == nil:
		found = discovered
		sources = append(sources, discovered.Vars()...)
	case errors.Is(err, collection.ErrNotFound):
		// A lone .http file with no collection around it is ordinary.
	default:
		return scopes{}, err
	}

	// 5. Environment.
	if opts.Environment != "" {
		if found == nil {
			return scopes{}, fmt.Errorf("--env %s needs a collection: no %s found above %s",
				opts.Environment, collection.ConfigName, file.Dir)
		}
		source, err := found.Environment(opts.Environment)
		if err != nil {
			return scopes{}, err
		}
		sources = append(sources, source)
	}

	// 6. File-level @name = value.
	sources = append(sources, httpfile.FileVars(file, path))

	// 8. Command line. Request-level vars (7) are layered per request.
	flagVars, err := parseVarFlags(opts.Vars)
	if err != nil {
		return scopes{}, err
	}
	sources = append(sources, httpfile.MapSource{Values: flagVars, Label: "--var"})

	namespaces, err := buildNamespaces(found)
	if err != nil {
		return scopes{}, err
	}

	result := scopes{
		resolver:   httpfile.Resolver{Sources: sources, Namespaces: namespaces},
		collection: found,
	}
	if found != nil {
		result.defaults = found.Defaults()
	}
	return result, nil
}

// buildNamespaces assembles the namespaced sources. These are not precedence
// levels: a {{secret:token}} never competes with an ordinary {{token}}.
func buildNamespaces(found *collection.Collection) (map[string]httpfile.Source, error) {
	namespaces := map[string]httpfile.Source{
		httpfile.EnvNamespace: httpfile.EnvSource{},
	}

	var (
		values map[string]string
		label  = "environment"
	)
	if found != nil {
		secrets, path, err := found.Secrets()
		if err != nil {
			return nil, err
		}
		if len(secrets) > 0 {
			values, label = secrets, path
		}
	}

	namespaces[httpfile.SecretNamespace] = secretSource{values: values, label: label}
	return namespaces, nil
}

// secretSource resolves {{secret:name}} from the gitignored secrets file, then
// from the environment as a CI escape hatch.
//
// The OS keychain is the intended first source and is not implemented yet;
// until it is, these two cover a developer machine and a pipeline.
type secretSource struct {
	values map[string]string
	label  string
}

func (s secretSource) Lookup(name string) (string, bool) {
	if value, ok := s.values[name]; ok {
		return value, true
	}
	return os.LookupEnv(name)
}

func (s secretSource) Origin() string { return s.label }

// withRequestVars layers a request's own "# @var" definitions between the file
// scope and the command line.
func withRequestVars(base httpfile.Resolver, request httpfile.Request, path string) httpfile.Resolver {
	values := make(map[string]string, len(request.Vars))
	for _, v := range request.Vars {
		values[v.Name] = v.Value
	}

	// The command line stays highest, so the request scope is inserted just
	// beneath it rather than appended.
	sources := make([]httpfile.Source, 0, len(base.Sources)+1)
	sources = append(sources, base.Sources[:len(base.Sources)-1]...)
	sources = append(sources, httpfile.MapSource{
		Values: values,
		Label:  fmt.Sprintf("%s:%d", path, request.Line),
	})
	sources = append(sources, base.Sources[len(base.Sources)-1])

	return httpfile.Resolver{Sources: sources, Namespaces: base.Namespaces}
}

// expandDefaults substitutes placeholders in a collection's inherited headers
// and auth.
//
// Without this an inherited `token = "{{secret:api_token}}"` would be sent
// literally — the placeholder text as the bearer token — which fails in a way
// that looks like a server problem rather than a configuration one.
func expandDefaults(x *httpfile.Expander, defaults collection.Defaults, line int) (collection.Defaults, error) {
	expanded := collection.Defaults{Headers: make(map[string]string, len(defaults.Headers))}

	for name, value := range defaults.Headers {
		text, err := x.Expand(value, line)
		if err != nil {
			return collection.Defaults{}, err
		}
		expanded.Headers[name] = text
	}

	expanded.Auth = defaults.Auth
	for _, field := range []*string{&expanded.Auth.Token, &expanded.Auth.Username, &expanded.Auth.Password} {
		text, err := x.Expand(*field, line)
		if err != nil {
			return collection.Defaults{}, err
		}
		*field = text
	}

	return expanded, nil
}

// warnSystemScoped reports a committed request resolving from a machine-local
// scope. It works here and fails for a teammate, which is a confusing failure
// to debug remotely — so it is allowed, and announced.
func warnSystemScoped(names []string) {
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	fmt.Fprintf(os.Stderr,
		"warning: %s resolved from machine-local %s; a teammate running this file will not have it\n",
		strings.Join(names, ", "), systemVarsPath())
}

func parseVarFlags(args []string) (map[string]string, error) {
	values := make(map[string]string, len(args))
	for _, arg := range args {
		name, value, err := httpfile.ParseVarFlag(arg)
		if err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, nil
}

func loadFlatIfPresent(path string) (map[string]string, error) {
	values, err := collection.LoadFlat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return values, err
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
// renders the result, reusing the rendering and exit-code rules the
// command-line path uses.
func sendResolved(resolved httpfile.Resolved, defaults collection.Defaults, opts *Options) error {
	request := resolved.Request

	url, err := core.GenerateURL(request.URL, opts.HTTP, opts.QueryParams)
	if err != nil {
		return err
	}

	built := core.NewRequest(request.Method, url)

	for key, value := range opts.Cookies {
		built.WithCookie(key, value)
	}

	// Layered least specific first: collection defaults, then -n flags, then
	// the headers written in the file. The file describes this request, so it
	// wins over anything inherited.
	for name, value := range defaults.Headers {
		built.WithHeader(name, value)
	}
	applyCollectionAuth(&built, defaults.Auth, opts)
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
	// A secret must not be printed back by -v. Masking here rather than at the
	// point of substitution keeps the request that goes on the wire correct
	// while the one that goes on the screen is not a credential leak.
	if !opts.ShowSecrets {
		renderOpts.Mask = resolved.Secrets
	}

	if result.Stream != nil {
		return streamResult(result, opts, renderOpts)
	}

	fmt.Println(render.Render(result, renderOpts))

	return checkStatus(result, opts)
}

// applyCollectionAuth applies a collection's [defaults.auth], unless the
// command line already supplied credentials.
func applyCollectionAuth(request *core.BaseRequest, credentials collection.Auth, opts *Options) {
	if len(opts.Auth) > 0 {
		return
	}

	switch strings.ToLower(credentials.Type) {
	case "bearer":
		if credentials.Token != "" {
			request.WithHeader("Authorization", "Bearer "+credentials.Token)
		}
	case "basic":
		if credentials.Username != "" {
			request.BasicAuth = auth.BaseAuth{
				Username: credentials.Username,
				Password: credentials.Password,
			}
		}
	}
}
