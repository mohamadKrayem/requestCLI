// Package render turns a core.Result into text for a terminal.
//
// Render is a pure function: same Result and same Options give the same string,
// with no global state and no terminal probing inside. That is what lets the
// output be tested without a TTY, and what a TUI needs in order to re-render on
// resize, fold and search.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mohamadkrayem/requestCLI/core"
)

// Options selects what to show and how to colour it.
type Options struct {
	// The three Show flags are a selection set and combine. When none is set,
	// everything is shown.
	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool

	// Color is resolved by the caller from the TTY, NO_COLOR and any flag.
	// Nothing below this point makes that decision for itself.
	Color bool

	// Style is the chroma style used for non-JSON syntax highlighting.
	Style string
}

// DefaultStyle is used when Options.Style is empty.
const DefaultStyle = "monokai"

// ANSI codes, used directly so that rendering stays a pure function. A colour
// library that consults global state would defeat the point.
const (
	ansiReset   = "\x1b[0m"
	ansiCyan    = "\x1b[36m"
	ansiHiBlue  = "\x1b[94m"
	ansiHiCyan  = "\x1b[96m"
	ansiHiWhite = "\x1b[97m"
)

// Render renders the parts of the result the caller asked to see.
func Render(r *core.Result, opts Options) string {
	if r == nil {
		return ""
	}
	if !opts.ShowStatus && !opts.ShowHeaders && !opts.ShowBody {
		opts.ShowStatus, opts.ShowHeaders, opts.ShowBody = true, true, true
	}

	var out strings.Builder
	if opts.ShowStatus {
		fmt.Fprintf(&out, "\n%s %s\n",
			colorize(r.Proto, ansiHiCyan, opts.Color),
			colorize(r.Status, ansiHiBlue, opts.Color))
	}
	if opts.ShowHeaders {
		out.WriteString(renderHeaders(r, opts))
	}
	if opts.ShowBody {
		out.WriteString(renderBody(r, opts))
	}

	return strings.TrimRight(out.String(), "\n")
}

// renderHeaders renders the response headers, one line per value.
//
// Keys are sorted so that repeated runs of the same request produce identical
// output; Go map iteration order is random, and snapshot diffing (and these
// tests) need the result to be stable.
func renderHeaders(r *core.Result, opts Options) string {
	keys := make([]string, 0, len(r.Headers))
	for key := range r.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out strings.Builder
	for _, key := range keys {
		for _, value := range r.Headers[key] {
			fmt.Fprintf(&out, "%s:   %s\n",
				colorize(key, ansiCyan, opts.Color),
				colorize(value, ansiHiWhite, opts.Color))
		}
	}
	out.WriteString("\n")
	return out.String()
}

// renderBody renders the response body according to its media type.
func renderBody(r *core.Result, opts Options) string {
	if len(r.Body) == 0 {
		return ""
	}

	mediaType := r.MediaType()

	if isBinary(r.Body, mediaType) {
		return binaryNotice(r.Body, mediaType)
	}

	if isJSON(mediaType) {
		if out, ok := renderJSON(r.Body, opts.Color); ok {
			return out
		}
		// The header claimed JSON but the payload is not parseable.
		// Showing the raw body beats failing the whole command.
		return string(r.Body)
	}

	if lexer, ok := lexerFor(mediaType); ok {
		return highlight(r.Body, lexer, opts)
	}
	return string(r.Body)
}

// colorize wraps s in an ANSI code, or returns it untouched when colour is off.
func colorize(s, code string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}
