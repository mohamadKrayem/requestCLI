package httpfile

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// placeholder matches {{name}}, capturing the name.
var placeholder = regexp.MustCompile(`\{\{\s*([^{}\s][^{}]*?)\s*\}\}`)

// Namespace prefixes. A reference carrying one resolves only from that
// namespace, never from the ordinary precedence chain.
//
// This is the point of the design: in Postman a {{token}} might come from any
// of five scopes and you cannot tell which. Making the origin part of the
// reference removes the ambiguity, and it means a secret can never silently
// shadow an ordinary variable — the failure mode that makes credential bugs
// hard to see.
const (
	SecretNamespace = "secret"
	EnvNamespace    = "env"
	// dynamicPrefix marks a built-in generated per request: {{$uuid}}.
	dynamicPrefix = "$"
)

// Source supplies variable values. Each scope in the precedence model is one
// Source, and Resolver consults them in order.
//
// Origin exists so `rq vars` can report where a value came from, the way
// `git config --list --show-origin` does. That is only possible if every
// source names itself.
type Source interface {
	Lookup(name string) (value string, ok bool)
	Origin() string
}

// MapSource is a Source backed by a map, with a fixed origin label.
type MapSource struct {
	Values map[string]string
	Label  string
	// System marks a machine-local scope. Resolving a committed request from
	// one is convenient but a reproducibility footgun: it works here and
	// fails for a teammate. Callers warn rather than forbid.
	System bool
}

// Lookup reports the value this source defines for name, if any.
func (m MapSource) Lookup(name string) (string, bool) {
	value, ok := m.Values[name]
	return value, ok
}

// Origin returns the label this source reports in diagnostics.
func (m MapSource) Origin() string { return m.Label }

// IsSystem reports whether this scope is machine-local.
func (m MapSource) IsSystem() bool { return m.System }

// EnvSource resolves {{env:NAME}} from the process environment.
type EnvSource struct{}

// Lookup reads the environment variable.
func (EnvSource) Lookup(name string) (string, bool) { return os.LookupEnv(name) }

// Origin names the process environment.
func (EnvSource) Origin() string { return "environment" }

// FileVars builds a Source from a file's @name definitions. A name defined
// twice takes its last value, matching how other clients read the format.
func FileVars(file *File, label string) MapSource {
	values := make(map[string]string, len(file.Vars))
	for _, v := range file.Vars {
		values[v.Name] = v.Value
	}
	return MapSource{Values: values, Label: label}
}

// Resolver substitutes {{placeholders}}.
//
// Sources are the ordinary precedence chain, consulted last to first, so the
// slice reads lowest precedence first and matches the order of the scope
// table. Namespaces are keyed by prefix and sit outside that chain entirely.
type Resolver struct {
	Sources    []Source
	Namespaces map[string]Source
}

// Resolution records where one variable's value came from.
type Resolution struct {
	Name   string
	Value  string
	Origin string
	// Secret marks a value from the secret namespace, so output can mask it.
	Secret bool
	// System marks a value from a machine-local scope, so the caller can warn.
	System bool
}

// UnknownVariableError reports a placeholder no source defines.
type UnknownVariableError struct {
	File string
	Name string
	Line int
}

func (e *UnknownVariableError) Error() string {
	return fmt.Sprintf("%s unknown variable {{%s}}", Position(e.File, e.Line), e.Name)
}

// Resolved is a request with every placeholder substituted, plus what the
// substitution touched.
type Resolved struct {
	Request Request
	// Secrets holds the values substituted from the secret namespace. They are
	// masked wherever the request is displayed.
	Secrets []string
	// SystemScoped names variables that resolved from a machine-local scope.
	SystemScoped []string
}

// Lookup returns the winning value for a name and the source that supplied it.
//
// A namespaced reference resolves only from its namespace. A $-prefixed one is
// generated and therefore has no source; callers use expansion for those.
func (r Resolver) Lookup(name string) (Resolution, bool) {
	if namespace, key, found := strings.Cut(name, ":"); found {
		source, ok := r.Namespaces[namespace]
		if !ok {
			return Resolution{}, false
		}
		value, ok := source.Lookup(key)
		if !ok {
			return Resolution{}, false
		}
		return Resolution{
			Name:   name,
			Value:  value,
			Origin: source.Origin(),
			Secret: namespace == SecretNamespace,
		}, true
	}

	for i := len(r.Sources) - 1; i >= 0; i-- {
		value, ok := r.Sources[i].Lookup(name)
		if !ok {
			continue
		}
		system := false
		if s, isSystem := r.Sources[i].(interface{ IsSystem() bool }); isSystem {
			system = s.IsSystem()
		}
		return Resolution{
			Name:   name,
			Value:  value,
			Origin: r.Sources[i].Origin(),
			System: system,
		}, true
	}
	return Resolution{}, false
}

// expansion carries the state of resolving one request.
//
// The dynamic cache is per request, which is what makes two {{$uuid}}
// references in the same request agree — a correlation id that differed
// between a header and the body it labels would be worse than useless.
type expansion struct {
	resolver Resolver
	dynamic  map[string]string
	secrets  map[string]bool
	system   map[string]bool
	file     string
}

func newExpansion(r Resolver, file string) *expansion {
	return &expansion{
		resolver: r,
		dynamic:  map[string]string{},
		secrets:  map[string]bool{},
		system:   map[string]bool{},
		file:     file,
	}
}

// expand substitutes every placeholder in s.
//
// Substitution is single-pass: a value that itself contains {{...}} is left
// alone. Recursive expansion invites cycles and lets an injected value quietly
// become a reference, and no other client of this format does it.
func (e *expansion) expand(s string, line int) (string, error) {
	var failure error

	out := placeholder.ReplaceAllStringFunc(s, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]

		if value, ok := e.dynamicValue(name); ok {
			return value
		}

		resolution, ok := e.resolver.Lookup(name)
		if !ok {
			if failure == nil {
				failure = &UnknownVariableError{File: e.file, Name: name, Line: line}
			}
			return match
		}
		if resolution.Secret && resolution.Value != "" {
			e.secrets[resolution.Value] = true
		}
		if resolution.System {
			e.system[name] = true
		}
		return resolution.Value
	})

	if failure != nil {
		return "", failure
	}
	return out, nil
}

// dynamicValue generates a built-in, caching it for the rest of the request.
func (e *expansion) dynamicValue(name string) (string, bool) {
	key, ok := strings.CutPrefix(name, dynamicPrefix)
	if !ok {
		return "", false
	}
	if cached, seen := e.dynamic[key]; seen {
		return cached, true
	}

	value, ok := generate(key)
	if !ok {
		return "", false
	}
	e.dynamic[key] = value
	return value, true
}

// generate produces one built-in value.
func generate(name string) (string, bool) {
	switch name {
	case "uuid":
		return uuidV4(), true
	case "timestamp":
		return strconv.FormatInt(time.Now().Unix(), 10), true
	case "randomInt":
		n, err := rand.Int(rand.Reader, big.NewInt(1<<31))
		if err != nil {
			return "", false
		}
		return n.String(), true
	default:
		return "", false
	}
}

// uuidV4 returns a random UUID.
//
// Written out rather than pulled in as a dependency: it is sixteen random
// bytes and two bit-twiddles, and crypto/rand is already imported.
func uuidV4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any supported platform; if it ever
		// does, a timestamp is a worse but honest fallback.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Expand substitutes every placeholder in s, without collecting secrets.
// It is the simple entry point for callers that are not resolving a request.
func (r Resolver) Expand(s string, line int) (string, error) {
	return newExpansion(r, "").expand(s, line)
}

// Expander expands a group of strings as one unit, accumulating what the
// substitutions touched.
//
// It exists because a request is not the only thing that carries placeholders:
// a collection's default headers and auth do too, and they must be expanded in
// the same pass. Sharing one Expander is also what makes a {{$uuid}} in an
// inherited header match the one in the body it labels, since the per-request
// cache lives here.
type Expander struct{ e *expansion }

// NewExpander starts an expansion pass. file names the document in error
// positions and may be empty.
func (r Resolver) NewExpander(file string) *Expander {
	return &Expander{e: newExpansion(r, file)}
}

// Expand substitutes every placeholder in s.
func (x *Expander) Expand(s string, line int) (string, error) {
	return x.e.expand(s, line)
}

// Secrets returns the values substituted from the secret namespace so far.
func (x *Expander) Secrets() []string { return keys(x.e.secrets) }

// SystemScoped returns the variables resolved from a machine-local scope.
func (x *Expander) SystemScoped() []string { return keys(x.e.system) }

// ExpandRequest substitutes throughout a request and reads any "< ./path"
// body. dir is the directory the document came from.
func (x *Expander) ExpandRequest(request Request, dir string) (Request, error) {
	e := x.e

	resolved := request
	var err error

	if resolved.URL, err = e.expand(request.URL, request.Line); err != nil {
		return Request{}, err
	}
	if resolved.Name, err = e.expand(request.Name, request.Line); err != nil {
		return Request{}, err
	}

	resolved.Headers = make([]Header, len(request.Headers))
	for i, header := range request.Headers {
		value, err := e.expand(header.Value, request.Line)
		if err != nil {
			return Request{}, err
		}
		resolved.Headers[i] = Header{Name: header.Name, Value: value}
	}

	switch {
	case request.BodyFile != "":
		path, err := e.expand(request.BodyFile, request.Line)
		if err != nil {
			return Request{}, err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return Request{}, fmt.Errorf("%s reading body file: %w", Position(e.file, request.Line), err)
		}
		resolved.Body = body
		resolved.BodyFile = path

	case len(request.Body) > 0:
		body, err := e.expand(string(request.Body), request.Line)
		if err != nil {
			return Request{}, err
		}
		resolved.Body = []byte(body)
	}

	return resolved, nil
}

// Resolve returns the request with every placeholder substituted and any
// "< ./path" body read from disk.
//
// It is the single-request convenience form of NewExpander followed by
// ExpandRequest; a caller that also has collection defaults to expand should
// use the Expander directly so both share one pass.
func (r Resolver) Resolve(request Request, dir, file string) (Resolved, error) {
	x := r.NewExpander(file)

	resolved, err := x.ExpandRequest(request, dir)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{
		Request:      resolved,
		Secrets:      x.Secrets(),
		SystemScoped: x.SystemScoped(),
	}, nil
}

func keys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// Names lists every placeholder used in a request, in first-appearance order.
func Names(request Request) []string {
	var (
		names []string
		seen  = map[string]bool{}
	)

	add := func(s string) {
		for _, match := range placeholder.FindAllStringSubmatch(s, -1) {
			if name := match[1]; !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	add(request.URL)
	for _, header := range request.Headers {
		add(header.Value)
	}
	add(string(request.Body))
	add(request.BodyFile)
	return names
}

// ParseVarFlag splits a "name=value" command-line variable.
func ParseVarFlag(arg string) (name, value string, err error) {
	name, value, found := strings.Cut(arg, "=")
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return "", "", fmt.Errorf("invalid --var %q; expected name=value", arg)
	}
	return name, value, nil
}
