package core

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
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

	// Stream is non-nil when the response is being consumed incrementally,
	// in which case Body is nil and the caller reads events from here until
	// io.EOF. The two are never both populated: a stream has no complete
	// body to hand over, which is the whole reason it is a stream.
	Stream *Stream

	// cleanup releases the connection and the request context. It is nil for
	// a buffered result, whose body Send has already closed.
	cleanup   func() error
	closeOnce sync.Once
	closeErr  error

	// Request is the request as it was actually sent, captured by Send.
	// Populated unconditionally; whether to display it is a rendering
	// decision (-v), not a transport one.
	Request *SentRequest
}

// Timing records how long the exchange took.
type Timing struct {
	// TTFB is the time from sending the request to the response headers
	// arriving. For a stream this is the only part Total cannot cover, since
	// the body is still being delivered.
	TTFB time.Duration
	// Total is the full exchange for a buffered response. For a stream it is
	// zero until the stream ends; read Stream.Stats instead while it runs.
	Total time.Duration
}

// Close releases the connection behind a streamed result. It is safe to call
// on any result, including a buffered one, and safe to call more than once —
// so a caller can always `defer result.Close()` without first checking which
// kind it got back.
//
// It is also safe to call concurrently, which the CLI relies on: a signal
// handler closes the result to unblock a read that is parked waiting for the
// next frame, racing the deferred close on the way out.
func (r *Result) Close() error {
	if r == nil || r.cleanup == nil {
		return nil
	}
	r.closeOnce.Do(func() { r.closeErr = r.cleanup() })
	return r.closeErr
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
	return mediaTypeOf(r.Headers)
}

// mediaTypeOf extracts the bare media type from a header set. It is shared by
// Result.MediaType and the streaming check in Send, which has to decide before
// there is a Result to ask.
func mediaTypeOf(headers http.Header) string {
	raw := headers.Get("Content-Type")
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

// EventStreamMediaType is the content type that turns on incremental
// rendering without the user asking for it.
const EventStreamMediaType = "text/event-stream"

// readResponseBody reads the body, decompressing it if the server encoded it.
func readResponseBody(res *http.Response) ([]byte, error) {
	reader, err := decompressingReader(res)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok && reader != res.Body {
		defer func() { _ = closer.Close() }()
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
