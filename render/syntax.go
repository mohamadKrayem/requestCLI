package render

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
)

// lexers maps a media type to a chroma lexer.
//
// chroma is already a dependency and ships lexers for all of these; before this
// map only HTML was highlighted and everything else fell through to raw text.
//
// JSON is deliberately absent: it goes through the dedicated raw-bytes printer
// in json.go, which is both faster and lossless. chroma is fine for the long
// tail, but JSON is the hot path.
var lexers = map[string]string{
	"text/html":             "html",
	"application/xhtml+xml": "html",

	"text/xml":             "xml",
	"application/xml":      "xml",
	"application/rss+xml":  "xml",
	"application/soap+xml": "xml",

	"text/yaml":          "yaml",
	"application/yaml":   "yaml",
	"application/x-yaml": "yaml",
	"text/x-yaml":        "yaml",

	"text/javascript":          "javascript",
	"application/javascript":   "javascript",
	"application/x-javascript": "javascript",

	"text/css": "css",

	"application/toml": "toml",
	"text/toml":        "toml",

	"application/sql": "sql",
	"text/x-sql":      "sql",

	"text/markdown":       "markdown",
	"application/graphql": "graphql",
}

// lexerFor picks a chroma lexer for a media type.
func lexerFor(mediaType string) (string, bool) {
	if lexer, ok := lexers[mediaType]; ok {
		return lexer, true
	}
	// Structured-syntax suffixes: application/atom+xml and friends.
	if strings.HasSuffix(mediaType, "+xml") {
		return "xml", true
	}
	return "", false
}

// highlight applies syntax highlighting, falling back to the plain body.
func highlight(body []byte, lexer string, opts Options) string {
	if !opts.Color {
		return string(body)
	}

	style := opts.Style
	if style == "" {
		style = DefaultStyle
	}

	var buf bytes.Buffer
	// terminal256 rather than the 8-colour "terminal" formatter: every terminal
	// this tool is likely to run in supports it, and it looks dramatically better.
	if err := quick.Highlight(&buf, string(body), lexer, "terminal256", style); err != nil {
		// Highlighting is cosmetic: never fail the command over it.
		return string(body)
	}
	return buf.String()
}
