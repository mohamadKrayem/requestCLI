// Package httpfile parses .http request files.
//
// The dialect is the one VS Code REST Client, the JetBrains HTTP Client and
// kulala.nvim already read, so files written for any of them work here and
// files written here work there. That compatibility is the whole reason for
// adopting the format instead of inventing one, and it is why this package
// tolerates constructs it does not itself use rather than rejecting them.
//
// It depends on nothing but the standard library. In particular it does not
// import core: a .http file is a source format, not a transport concern, and
// keeping the two apart is what lets the parser be tested without building a
// request and lets core stay usable by a front-end that never reads a file.
package httpfile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// separator starts a new request. Anything after it on the line names the
// request that follows.
const separator = "###"

// Header is one header line, kept in file order. Duplicates are preserved:
// sending two Accept headers is legitimate and the file is the record of
// intent.
type Header struct {
	Name  string
	Value string
}

// Request is one request from a .http file, before variable resolution.
//
// Every field may still contain {{placeholders}}; Resolve substitutes them.
// Line numbers are retained so an error can point at the source.
type Request struct {
	// Name comes from "### name" or an explicit "# @name value" directive.
	// It is empty for an unnamed request, which is addressed by position.
	Name    string
	Method  string
	URL     string
	Headers []Header
	Body    []byte

	// BodyFile is set when the body was written as "< ./path", in which case
	// Body is empty until the file is read. The path is relative to the .http
	// file, matching every other client that reads this dialect.
	BodyFile string

	// Line is the 1-based line of the request line, used in error messages.
	Line int
}

// File is a parsed .http file.
type File struct {
	// Vars holds file-level "@name = value" definitions, in the order they
	// appeared. They are the lowest-precedence source of variables.
	Vars []Var
	// Requests are in file order.
	Requests []Request
	// Dir is the directory the file was read from, used to resolve a relative
	// "< ./body.json". Empty when parsed from a reader.
	Dir string
	// Path is the file the document came from, used in error positions.
	// Empty when parsed from a reader.
	Path string
}

// Var is one "@name = value" definition.
type Var struct {
	Name  string
	Value string
	Line  int
}

// ParseError reports a malformed file, with the line responsible.
//
// The position is the point: a parse failure that only says what went wrong
// makes the reader search for where, which is the complaint the JSON parse
// errors already fixed elsewhere in this tool.
type ParseError struct {
	// File is set when the document came from ParseFile, empty when it came
	// from a reader.
	File string
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return Position(e.File, e.Line) + " " + e.Msg
}

// Position formats a source location as "file:line:", or "line N:" when the
// document did not come from a file.
//
// The file:line: form is what editors and terminals turn into a clickable
// jump, which is worth more here than prose: the whole feature is about files
// the user is editing.
func Position(file string, line int) string {
	if file == "" {
		return fmt.Sprintf("line %d:", line)
	}
	return fmt.Sprintf("%s:%d:", file, line)
}

// ParseFile reads and parses a .http file.
func ParseFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	file, err := Parse(f)
	if err != nil {
		// Name the file, so an error says where as well as what.
		var parseErr *ParseError
		if errors.As(err, &parseErr) {
			parseErr.File = path
		}
		return nil, err
	}
	file.Dir = filepath.Dir(path)
	file.Path = path
	return file, nil
}

// Parse parses a .http document.
func Parse(r io.Reader) (*File, error) {
	p := &parser{scanner: bufio.NewScanner(r), file: &File{}}
	p.scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	if err := p.run(); err != nil {
		return nil, err
	}
	return p.file, nil
}

// maxLine caps one line of a .http file. A body line can legitimately be long
// — a pasted JSON document on one line — but not unbounded.
const maxLine = 4 << 20 // 4 MiB

// section is where in a request the parser currently is.
type section int

const (
	// betweenRequests is before any request line has been seen, where
	// separators, comments and @var definitions are legal.
	betweenRequests section = iota
	// inHeaders is after the request line, before the blank line.
	inHeaders
	// inBody is after the blank line.
	inBody
)

type parser struct {
	scanner *bufio.Scanner
	file    *File

	line    int
	current *Request
	// pendingName is a name taken from a "### name" separator, applied to the
	// next request line encountered.
	pendingName string
	body        []string
	state       section
}

func (p *parser) run() error {
	for p.scanner.Scan() {
		p.line++
		if err := p.consume(p.scanner.Text()); err != nil {
			return err
		}
	}
	if err := p.scanner.Err(); err != nil {
		return fmt.Errorf("reading: %w", err)
	}
	p.finish()
	return nil
}

// consume handles one line.
func (p *parser) consume(raw string) error {
	trimmed := strings.TrimSpace(raw)

	// A separator ends the current request wherever it appears, including
	// inside a body: "###" is the one token the dialect reserves absolutely.
	if strings.HasPrefix(trimmed, separator) {
		p.finish()
		p.pendingName = strings.TrimSpace(strings.TrimPrefix(trimmed, separator))
		p.state = betweenRequests
		return nil
	}

	// Body lines are taken literally: a "#" inside a JSON string is data, and
	// a blank line inside a body is part of it.
	if p.state == inBody {
		p.body = append(p.body, raw)
		return nil
	}

	if directive, ok := parseDirective(trimmed); ok {
		return p.applyDirective(directive)
	}

	if isComment(trimmed) {
		return nil
	}

	if trimmed == "" {
		// A blank line after the headers opens the body. Before a request
		// line it is just spacing.
		if p.state == inHeaders {
			p.state = inBody
		}
		return nil
	}

	if p.state == betweenRequests {
		return p.startRequest(trimmed)
	}
	return p.addHeader(trimmed)
}

// startRequest parses a request line: "METHOD url [HTTP/1.1]", or a bare URL,
// which the dialect treats as GET.
func (p *parser) startRequest(line string) error {
	fields := strings.Fields(line)

	method := http.MethodGet
	rest := fields
	if len(fields) > 1 && isMethod(fields[0]) {
		method = strings.ToUpper(fields[0])
		rest = fields[1:]
	}

	if len(rest) == 0 {
		return &ParseError{Line: p.line, Msg: "request line has no URL"}
	}

	// A trailing "HTTP/1.1" is part of the grammar and carries no information
	// we act on: the transport decides the version.
	url := rest[0]
	if len(rest) > 1 && !strings.HasPrefix(strings.ToUpper(rest[1]), "HTTP/") {
		return &ParseError{
			Line: p.line,
			Msg:  fmt.Sprintf("unexpected %q after the URL; a request line is METHOD URL [HTTP/1.1]", rest[1]),
		}
	}

	p.current = &Request{
		Name:   p.pendingName,
		Method: method,
		URL:    url,
		Line:   p.line,
	}
	p.pendingName = ""
	p.state = inHeaders
	return nil
}

// addHeader parses a "Name: value" line.
func (p *parser) addHeader(line string) error {
	name, value, found := strings.Cut(line, ":")
	if !found {
		return &ParseError{
			Line: p.line,
			Msg:  fmt.Sprintf("expected a header of the form Name: value, got %q", line),
		}
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return &ParseError{Line: p.line, Msg: "header name is empty"}
	}

	p.current.Headers = append(p.current.Headers, Header{
		Name:  name,
		Value: strings.TrimSpace(value),
	})
	return nil
}

// finish closes the request being accumulated, if any.
func (p *parser) finish() {
	if p.current == nil {
		p.body = nil
		return
	}

	body := strings.Join(p.body, "\n")
	body = strings.Trim(body, "\n")

	// "< ./path" as the whole body reads it from a file. The form is part of
	// the dialect and is how anyone keeps a large payload out of the request
	// file.
	if path, ok := strings.CutPrefix(strings.TrimSpace(body), "<"); ok && !strings.Contains(body, "\n") {
		p.current.BodyFile = strings.TrimSpace(path)
	} else if body != "" {
		p.current.Body = []byte(body)
	}

	p.file.Requests = append(p.file.Requests, *p.current)
	p.current = nil
	p.body = nil
}

// directive is a "# @name value" annotation. The dialect puts these in
// comments so that parsers which do not understand them simply skip them,
// which is what makes the format extensible without forking it.
type directive struct {
	key   string
	value string
}

// parseDirective recognises "@name = value" and "# @name value".
func parseDirective(trimmed string) (directive, bool) {
	body := trimmed
	// A directive may be written inside a comment, which is how request-level
	// annotations stay readable to other clients.
	for _, prefix := range []string{"#", "//"} {
		if stripped, ok := strings.CutPrefix(body, prefix); ok {
			body = strings.TrimSpace(stripped)
			break
		}
	}

	rest, ok := strings.CutPrefix(body, "@")
	if !ok {
		return directive{}, false
	}

	// Both "@name = value" and "@name value" appear in the wild.
	key, value, found := strings.Cut(rest, "=")
	if !found {
		key, value, found = strings.Cut(rest, " ")
		if !found {
			return directive{key: strings.TrimSpace(rest)}, true
		}
	}
	return directive{key: strings.TrimSpace(key), value: strings.TrimSpace(value)}, true
}

// applyDirective handles the directives this slice understands and ignores the
// rest, so a file carrying annotations for another client still parses.
func (p *parser) applyDirective(d directive) error {
	if d.key == "" {
		return &ParseError{Line: p.line, Msg: "directive has no name"}
	}

	if strings.EqualFold(d.key, "name") {
		// A name given before the request line names the request that
		// follows; one given inside a request renames it.
		if p.current != nil {
			p.current.Name = d.value
		} else {
			p.pendingName = d.value
		}
		return nil
	}

	// Any other @name is a file-level variable, but only before a request
	// line: inside a request it is an annotation for a feature this slice
	// does not implement, and silently treating it as a variable would be
	// worse than ignoring it.
	if p.state == betweenRequests {
		p.file.Vars = append(p.file.Vars, Var{Name: d.key, Value: d.value, Line: p.line})
	}
	return nil
}

// isComment reports whether a line is a comment. "###" is handled before this
// is reached, since it is a separator rather than a comment.
func isComment(trimmed string) bool {
	return strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//")
}

// methods are the verbs a request line may open with. A token that is not one
// of these is treated as a URL, so a bare "example.com/path" line still parses
// as a GET the way the dialect intends.
var methods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodHead: true,
	http.MethodOptions: true, http.MethodTrace: true, http.MethodConnect: true,
}

func isMethod(token string) bool { return methods[strings.ToUpper(token)] }
