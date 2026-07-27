// Package core builds and sends HTTP requests and returns structured results.
//
// Nothing in this package renders, colorizes or writes to a terminal. It must
// never import render/ or any terminal package: every front-end (CLI, TUI,
// editor plugin) depends on core, and none of them can share a result that has
// already been turned into a coloured string.
package core

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"time"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/formats"
)

// Version is reported in the default User-Agent header.
const Version = "0.3.0"

// DefaultTimeout bounds a whole request/response cycle when none is given.
const DefaultTimeout = 30 * time.Second

// BaseRequest is the base request object.
type BaseRequest struct {
	Method        string
	URL           string
	Headers       map[string]any
	Cookies       map[string]string
	Body          string
	BasicAuth     auth.BaseAuth
	MultipartBody io.Reader
	Writer        *multipart.Writer

	// Unset holds header names (canonical form) that were deliberately
	// removed with WithoutHeader. The zero value is a nil map, which every
	// read here tolerates, so a BaseRequest built as a struct literal (not
	// via NewRequest) still works.
	Unset map[string]bool
}

// SentRequest is the request as it was actually put on the wire, as structured
// data. It exists so a front-end can display the request without core printing
// anything: -v is a rendering problem, not a transport concern.
type SentRequest struct {
	Method  string
	URL     string
	Proto   string
	Headers http.Header // final, including defaults, auth and cookies
	Body    []byte      // nil when there was no body
}

// TransportError reports a failure to complete the exchange — DNS, connect,
// TLS or timeout. It is distinct from a request that could not be built, so a
// caller can choose a different exit code for each.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// SendOptions carries the per-invocation transport settings.
//
// Output selection is deliberately absent: what to display is a rendering
// decision and belongs to render.Options, not to the transport.
type SendOptions struct {
	Redirect bool
	// Insecure disables TLS certificate verification. Off by default.
	Insecure bool
	Timeout  time.Duration
}

// NewRequest creates a new BaseRequest object.
func NewRequest(method, url string) BaseRequest {
	return BaseRequest{
		Method:  method,
		URL:     url,
		Headers: make(map[string]any),
		Cookies: make(map[string]string),
	}
}

// WithHeader adds a single header to the request.
//
// The key is canonicalised the same way http.Header.Set already canonicalises
// on the wire, so a later WithoutHeader("user-agent") reliably matches a
// header set here as "User-Agent".
func (req *BaseRequest) WithHeader(key string, value string) *BaseRequest {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	key = http.CanonicalHeaderKey(key)
	req.Headers[key] = value
	// Setting a header explicitly cancels an earlier unset, so the last thing
	// the caller asked for is what happens. delete on a nil map is a no-op.
	delete(req.Unset, key)
	return req
}

// WithoutHeader marks a header as deliberately unset: it is removed if
// already present, and addDefaultHeaders will never fill it back in.
func (req *BaseRequest) WithoutHeader(key string) *BaseRequest {
	key = http.CanonicalHeaderKey(key)
	if req.Unset == nil {
		req.Unset = make(map[string]bool)
	}
	req.Unset[key] = true
	delete(req.Headers, key)
	return req
}

// WithHeaders merges the key/values of a JSON document into the request headers.
func (req *BaseRequest) WithHeaders(jsonData formats.JSON) error {
	jsonMap, err := jsonData.ToMap()
	if err != nil {
		return fmt.Errorf("reading headers: %w", err)
	}
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	for key, value := range jsonMap {
		req.Headers[key] = value
	}
	return nil
}

// WithHeadersMap merges the key/values of a map into the request headers.
func (req *BaseRequest) WithHeadersMap(headersMap map[string]string) *BaseRequest {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	for key, value := range headersMap {
		req.Headers[key] = value
	}
	return req
}

// WithCookie adds a single cookie to the request.
func (req *BaseRequest) WithCookie(key string, value string) *BaseRequest {
	if req.Cookies == nil {
		req.Cookies = make(map[string]string)
	}
	req.Cookies[key] = value
	return req
}

// Send sends the request and returns the structured result.
func (req *BaseRequest) Send(opts SendOptions) (*Result, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			if opts.Redirect {
				return nil
			}
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			// Verification stays on unless the user explicitly passes --insecure.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.Insecure},
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// The body is materialised into []byte before the request is built, rather
	// than streamed straight from req.MultipartBody, so the exact bytes put on
	// the wire can be captured into SentRequest below. The multipart body is
	// already a *bytes.Buffer, so this is not a streaming regression.
	var bodyBytes []byte
	hasBody := false
	if req.Writer != nil {
		b, err := io.ReadAll(req.MultipartBody)
		if err != nil {
			return nil, fmt.Errorf("reading multipart body for %s %s: %w", req.Method, req.URL, err)
		}
		bodyBytes = b
		hasBody = true
	} else if req.Body != "" {
		bodyBytes = []byte(req.Body)
		hasBody = true
	}

	var bodyReader io.Reader
	if hasBody {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	reqHTTP, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building %s request for %s: %w", req.Method, req.URL, err)
	}

	req.addDefaultHeaders(hasBody)
	for key, value := range req.Headers {
		// A key set through WithHeadersMap/WithHeaders keeps whatever case the
		// caller gave it, so the Unset check must canonicalise here too.
		if req.Unset[http.CanonicalHeaderKey(key)] {
			continue
		}
		reqHTTP.Header.Set(key, fmt.Sprintf("%v", value))
	}
	// net/http is alone in filling in its own default (Go-http-client/x.y)
	// when User-Agent is entirely absent from reqHTTP.Header, so skipping the
	// copy above suppresses every other unset header but not this one.
	// Explicitly setting it to "" makes net/http both skip its default and
	// omit the header from the wire, rather than sending it empty.
	if req.Unset["User-Agent"] {
		reqHTTP.Header.Set("User-Agent", "")
	}

	if req.BasicAuth.Username != "" {
		reqHTTP.SetBasicAuth(req.BasicAuth.Username, req.BasicAuth.Password)
	}

	for key, value := range req.Cookies {
		reqHTTP.AddCookie(&http.Cookie{Name: key, Value: value})
	}

	// Captured after auth and cookies are applied, so the header set here is
	// exactly what reached the wire. Populated unconditionally: -v, --offline
	// and "copy as curl" are all rendering concerns, not transport ones.
	sentRequest := &SentRequest{
		Method:  req.Method,
		URL:     reqHTTP.URL.String(),
		Proto:   reqHTTP.Proto,
		Headers: reqHTTP.Header.Clone(),
		Body:    bodyBytes,
	}

	start := time.Now()
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return nil, &TransportError{Err: fmt.Errorf("sending %s %s: %w", req.Method, req.URL, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	result, err := NewResult(resp)
	if err != nil {
		return nil, err
	}
	result.Timing.Total = time.Since(start)
	result.Request = sentRequest
	return result, nil
}

// addDefaultHeaders fills in headers the user did not set explicitly, and
// never fills in one the user explicitly unset with WithoutHeader.
func (req *BaseRequest) addDefaultHeaders(hasBody bool) {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	req.setIfAbsent("Accept", "*/*")
	req.setIfAbsent("User-Agent", "rq/"+Version)
	// Requested explicitly because NewResult decodes brotli itself, which
	// net/http does not do.
	req.setIfAbsent("Accept-Encoding", "gzip, deflate, br")

	// A Content-Type on a bodyless request is meaningless and confuses some servers.
	if hasBody {
		req.setIfAbsent("Content-Type", "application/json")
	}
}

// setIfAbsent fills in a default header unless the caller already set it or
// deliberately unset it. req.Unset may be nil; reading a nil map is a safe
// no-op that reports "nothing unset".
func (req *BaseRequest) setIfAbsent(key, value string) {
	if req.Unset[key] {
		return
	}
	if _, ok := req.Headers[key]; !ok {
		req.Headers[key] = value
	}
}
