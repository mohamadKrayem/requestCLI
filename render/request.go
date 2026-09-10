package render

import (
	"fmt"
	"mime"
	"net/url"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/mohamadkrayem/requestCLI/core"
)

// Request renders the request that was sent. Exported so a front-end can
// show a request without a response (--offline, "copy as curl").
//
// The request line is "Method RequestURI Proto". Host is emitted first,
// derived from the URL; the remaining header keys are sorted, matching the
// response renderer's stability rule. The body goes through the same
// media-type pipeline as a response body, keyed off the request's own
// Content-Type.
func Request(r *core.SentRequest, opts Options) string {
	if r == nil {
		return ""
	}

	requestURI := r.URL
	host := ""
	if parsed, err := url.Parse(r.URL); err == nil {
		host = parsed.Host
		requestURI = parsed.RequestURI()
	}

	var out strings.Builder
	fmt.Fprintf(&out, "%s %s %s\n",
		colorize(r.Method, ansiHiCyan, opts.Color),
		colorize(requestURI, ansiHiWhite, opts.Color),
		colorize(r.Proto, ansiHiCyan, opts.Color))

	if host != "" {
		fmt.Fprintf(&out, "%s:   %s\n",
			colorize("Host", ansiCyan, opts.Color),
			colorize(host, ansiHiWhite, opts.Color))
	}

	keys := make([]string, 0, len(r.Headers))
	for key := range r.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range r.Headers[key] {
			fmt.Fprintf(&out, "%s:   %s\n",
				colorize(key, ansiCyan, opts.Color),
				colorize(value, ansiHiWhite, opts.Color))
		}
	}

	if len(r.Body) > 0 {
		out.WriteString("\n")
		out.WriteString(renderBodyBytes(r.Body, requestMediaType(r), opts))
	}

	return out.String()
}

// requestMediaType returns the request's own Content-Type, with any
// parameters stripped, lowercased — the same shape core.Result.MediaType
// returns for a response, but read from a plain http.Header rather than a
// Result.
func requestMediaType(r *core.SentRequest) string {
	raw := r.Headers.Get("Content-Type")
	if raw == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		mediaType, _, _ = strings.Cut(raw, ";")
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

// StyleNames returns the available chroma style names, for shell completion.
// It lives here because render is the only package that may import chroma.
func StyleNames() []string {
	return styles.Names()
}
