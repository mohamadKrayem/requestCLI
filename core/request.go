// Package core builds and sends HTTP requests and returns structured results.
//
// Nothing in this package renders, colorizes or writes to a terminal. It must
// never import render/ or any terminal package: every front-end (CLI, TUI,
// editor plugin) depends on core, and none of them can share a result that has
// already been turned into a coloured string.
package core

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/formats"
)

// Version is reported in the default User-Agent header.
const Version = "0.2.0"

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
}

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
func (req *BaseRequest) WithHeader(key string, value string) *BaseRequest {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	req.Headers[key] = value
	return req
}

// WithHeaders merges the key/values of a JSON document into the request headers.
func (req *BaseRequest) WithHeaders(jsonData formats.Json) error {
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
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
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

	var bodyReader io.Reader
	hasBody := false
	if req.Writer != nil {
		bodyReader = req.MultipartBody
		hasBody = true
	} else if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
		hasBody = true
	}

	reqHttp, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building %s request for %s: %w", req.Method, req.URL, err)
	}

	req.addDefaultHeaders(hasBody)
	for key, value := range req.Headers {
		reqHttp.Header.Set(key, fmt.Sprintf("%v", value))
	}

	if req.BasicAuth.Username != "" {
		reqHttp.SetBasicAuth(req.BasicAuth.Username, req.BasicAuth.Password)
	}

	for key, value := range req.Cookies {
		reqHttp.AddCookie(&http.Cookie{Name: key, Value: value})
	}

	start := time.Now()
	resp, err := client.Do(reqHttp)
	if err != nil {
		return nil, fmt.Errorf("sending %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	result, err := NewResult(resp)
	if err != nil {
		return nil, err
	}
	result.Timing.Total = time.Since(start)
	return result, nil
}

// addDefaultHeaders fills in headers the user did not set explicitly.
func (req *BaseRequest) addDefaultHeaders(hasBody bool) {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	setIfAbsent(req.Headers, "Accept", "*/*")
	setIfAbsent(req.Headers, "User-Agent", "requestCLI/"+Version)
	// Requested explicitly because NewResult decodes brotli itself, which
	// net/http does not do.
	setIfAbsent(req.Headers, "Accept-Encoding", "gzip, deflate, br")

	// A Content-Type on a bodyless request is meaningless and confuses some servers.
	if hasBody {
		setIfAbsent(req.Headers, "Content-Type", "application/json")
	}
}

func setIfAbsent(headers map[string]any, key, value string) {
	if _, ok := headers[key]; !ok {
		headers[key] = value
	}
}
