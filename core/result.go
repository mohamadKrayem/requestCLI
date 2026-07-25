package core

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/dsnet/compress/brotli"
)

// Result is one completed exchange, as structured data.
//
// Body holds the decompressed bytes exactly as the server sent them. It is
// deliberately never decoded here: decoding JSON in order to display it is what
// silently destroyed number precision, key order and duplicate keys in the
// previous design.
type Result struct {
	Proto      string
	Status     string // "200 OK"
	StatusCode int
	Headers    http.Header
	Body       []byte
	Timing     Timing
}

// Timing records how long the exchange took.
type Timing struct {
	Total time.Duration
}

// NewResult reads and decompresses a response into a Result.
//
// It does not close the body; the caller that issued the request owns it.
func NewResult(res *http.Response) (*Result, error) {
	result := &Result{
		Proto:      res.Proto,
		Status:     res.Status,
		StatusCode: res.StatusCode,
		Headers:    res.Header,
	}

	if res.Body != nil {
		body, err := readResponseBody(res)
		if err != nil {
			return nil, err
		}
		result.Body = body
	}
	return result, nil
}

// MediaType returns the Content-Type with any parameters stripped, lowercased.
//
// It returns an empty string when the header is absent or unparseable, which
// callers treat as "sniff it instead".
func (r *Result) MediaType() string {
	raw := r.Headers.Get("Content-Type")
	if raw == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		// Tolerate a malformed header by taking the part before the first ';'.
		mediaType, _, _ = strings.Cut(raw, ";")
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

// readResponseBody reads the body, decompressing it if the server encoded it.
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
