// Package response renders HTTP responses for terminal output.
package response

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/dsnet/compress/brotli"
	"github.com/fatih/color"
	"github.com/mohamadkrayem/requestCLI/formats"
)

type Response struct {
	Proto   string
	Status  string
	Headers string
	Body    string
}

// NewResponse renders the parts of the response the user asked to see.
//
// The three flags are a selection set, so they combine: -H -B shows headers and
// body. Passing none shows everything.
func NewResponse(httpRes *http.Response, showStatus, showHeaders, showBody bool) (*Response, error) {
	if !showStatus && !showHeaders && !showBody {
		showStatus, showHeaders, showBody = true, true, true
	}

	var response Response
	if showStatus {
		response.Proto = httpRes.Proto
		response.Status = httpRes.Status
	}
	if showHeaders {
		response.Headers = storeColorizedHeaders(httpRes)
	}
	if showBody {
		body, err := storeColorizedBody(httpRes)
		if err != nil {
			return nil, err
		}
		response.Body = body
	}
	return &response, nil
}

// PrintResponse prints the response to the console.
func (res *Response) PrintResponse() {
	statusColor := color.New(color.FgHiBlue).SprintFunc()
	protoColor := color.New(color.FgHiCyan).SprintFunc()

	var out strings.Builder
	if res.Status != "" {
		fmt.Fprintf(&out, "\n%s %s\n", protoColor(res.Proto), statusColor(res.Status))
	}
	out.WriteString(res.Headers)
	out.WriteString(res.Body)

	fmt.Println(strings.TrimRight(out.String(), "\n"))
}

// storeColorizedHeaders renders the headers in a colorized way.
func storeColorizedHeaders(res *http.Response) string {
	keyColor := color.New(color.FgCyan).SprintFunc()
	valColor := color.New(color.FgHiWhite).SprintFunc()

	var resSTR strings.Builder
	for key, values := range res.Header {
		for _, v := range values {
			fmt.Fprintf(&resSTR, "%s:   %s\n", keyColor(key), valColor(v))
		}
	}
	resSTR.WriteString("\n")
	return resSTR.String()
}

// storeColorizedBody renders the body, colorizing it according to its content type.
func storeColorizedBody(res *http.Response) (string, error) {
	if res.Body == nil {
		return "", nil
	}

	body, err := readResponseBody(res)
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", nil
	}

	contentType := res.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/html"):
		return colorizeHTML(body), nil

	case strings.Contains(contentType, "json"):
		colorized, err := colorizeJSON(body)
		if err != nil {
			// The header claimed JSON but the payload is not parseable.
			// Showing the raw body beats failing the whole command.
			return string(body), nil
		}
		return colorized, nil

	default:
		return string(body), nil
	}
}

// colorizeJSON pretty-prints and colorizes a JSON payload.
func colorizeJSON(body []byte) (string, error) {
	resJS, err := formats.NewJson(string(body))
	if err != nil {
		return "", err
	}
	return resJS.GetColorizedJSON()
}

// colorizeHTML applies syntax highlighting to an HTML payload.
func colorizeHTML(body []byte) string {
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, string(body), "html", "terminal", "monokai"); err != nil {
		// Highlighting is cosmetic: fall back to the plain body.
		return string(body)
	}
	return buf.String()
}

// readResponseBody reads the body, decompressing it if the server encoded it.
//
// It does not close the body; the caller that issued the request owns it.
func readResponseBody(res *http.Response) ([]byte, error) {
	reader, err := decompressingReader(res)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != res.Body {
		defer closer.Close()
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return body, nil
}

// decompressingReader wraps the body in a decoder for the advertised encoding.
func decompressingReader(res *http.Response) (io.Reader, error) {
	switch strings.TrimSpace(strings.ToLower(res.Header.Get("Content-Encoding"))) {
	case "gzip":
		reader, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding gzip response: %w", err)
		}
		return reader, nil

	case "deflate":
		return flate.NewReader(res.Body), nil

	case "br":
		reader, err := brotli.NewReader(res.Body, &brotli.ReaderConfig{})
		if err != nil {
			return nil, fmt.Errorf("decoding brotli response: %w", err)
		}
		return reader, nil

	default:
		return res.Body, nil
	}
}
