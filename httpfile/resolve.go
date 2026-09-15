package httpfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// placeholder matches {{name}}, capturing the name.
//
// The name is deliberately permissive: it accepts the namespaced forms the
// full model will use ({{secret:token}}, {{$uuid}}) so that a file using them
// produces "unknown variable secret:token" rather than a confusing parse
// failure, and starts resolving the day those sources exist.
var placeholder = regexp.MustCompile(`\{\{\s*([^{}\s][^{}]*?)\s*\}\}`)

// Source supplies variable values. Each scope in the eventual precedence model
// — system file, collection defaults, environment, file, request, command line
// — is one Source, and Resolver consults them in order.
//
// Origin exists so `rq vars` can report where a value came from, the way
// `git config --list --show-origin` does. That is only possible if every
// source names itself, which is why it is on the interface rather than bolted
// on later.
type Source interface {
	// Lookup returns the value and true when this source defines name.
	Lookup(name string) (value string, ok bool)
	// Origin describes this source for diagnostics, e.g. "api.http line 3"
	// or "--var".
	Origin() string
}

// MapSource is a Source backed by a map, with a fixed origin label.
type MapSource struct {
	Values map[string]string
	Label  string
}

// Lookup reports the value this source defines for name, if any.
func (m MapSource) Lookup(name string) (string, bool) {
	value, ok := m.Values[name]
	return value, ok
}

// Origin returns the label this source reports in diagnostics.
func (m MapSource) Origin() string { return m.Label }

// FileVars builds a Source from a file's @name definitions. A name defined
// twice takes its last value, matching how the other clients read the format.
func FileVars(file *File, label string) MapSource {
	values := make(map[string]string, len(file.Vars))
	for _, v := range file.Vars {
		values[v.Name] = v.Value
	}
	return MapSource{Values: values, Label: label}
}

// Resolver substitutes {{placeholders}} from an ordered list of sources.
//
// Sources are consulted last to first, so the slice reads lowest precedence
// first and matches the order of the table in the design notes. Adding
// environments, collection defaults or a keychain later means inserting a
// Source at the right index and changing nothing else.
type Resolver struct {
	Sources []Source
}

// Resolution records where one variable's value came from.
type Resolution struct {
	Name   string
	Value  string
	Origin string
}

// Lookup returns the winning value for a name and the source that supplied it.
func (r Resolver) Lookup(name string) (Resolution, bool) {
	for i := len(r.Sources) - 1; i >= 0; i-- {
		if value, ok := r.Sources[i].Lookup(name); ok {
			return Resolution{Name: name, Value: value, Origin: r.Sources[i].Origin()}, true
		}
	}
	return Resolution{}, false
}

// UnknownVariableError reports a placeholder no source defines.
//
// It names the variable and the line rather than leaving {{base_url}} in the
// URL to fail later as a confusing connection error.
type UnknownVariableError struct {
	// File is set by the caller that knows which document this came from.
	File string
	Name string
	Line int
}

func (e *UnknownVariableError) Error() string {
	return fmt.Sprintf("%s unknown variable {{%s}}", Position(e.File, e.Line), e.Name)
}

// Expand substitutes every placeholder in s.
//
// Substitution is single-pass: a value that itself contains {{...}} is left
// alone rather than expanded again. Recursive expansion invites cycles and an
// injected value that quietly becomes a reference, and no client of this
// format does it.
func (r Resolver) Expand(s string, line int) (string, error) {
	var unknown error

	out := placeholder.ReplaceAllStringFunc(s, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]
		resolution, ok := r.Lookup(name)
		if !ok {
			if unknown == nil {
				unknown = &UnknownVariableError{Name: name, Line: line}
			}
			return match
		}
		return resolution.Value
	})

	if unknown != nil {
		return "", unknown
	}
	return out, nil
}

// Resolve returns a copy of the request with every placeholder substituted and
// any "< ./path" body read from disk.
//
// dir is the directory the file came from; a body path is resolved against it,
// matching every other client that reads this dialect. An absolute path is
// used as given. file names the document in error positions and may be empty.
func (r Resolver) Resolve(request Request, dir, file string) (Request, error) {
	resolved, err := r.resolve(request, dir, file)
	if err != nil {
		var unknown *UnknownVariableError
		if errors.As(err, &unknown) {
			unknown.File = file
		}
		return Request{}, err
	}
	return resolved, nil
}

func (r Resolver) resolve(request Request, dir, file string) (Request, error) {
	resolved := request

	var err error
	if resolved.URL, err = r.Expand(request.URL, request.Line); err != nil {
		return Request{}, err
	}
	if resolved.Name, err = r.Expand(request.Name, request.Line); err != nil {
		return Request{}, err
	}

	resolved.Headers = make([]Header, len(request.Headers))
	for i, header := range request.Headers {
		value, err := r.Expand(header.Value, request.Line)
		if err != nil {
			return Request{}, err
		}
		resolved.Headers[i] = Header{Name: header.Name, Value: value}
	}

	if request.BodyFile != "" {
		path, err := r.Expand(request.BodyFile, request.Line)
		if err != nil {
			return Request{}, err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return Request{}, fmt.Errorf("%s reading body file: %w", Position(file, request.Line), err)
		}
		resolved.Body = body
		resolved.BodyFile = path
		return resolved, nil
	}

	if len(request.Body) > 0 {
		body, err := r.Expand(string(request.Body), request.Line)
		if err != nil {
			return Request{}, err
		}
		resolved.Body = []byte(body)
	}
	return resolved, nil
}

// Names lists every placeholder used in a request, in first-appearance order.
// It is what lets a caller report all the variables a file needs.
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
