// Package requests builds and sends HTTP requests.
package requests

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/formats"
	"github.com/mohamadkrayem/requestCLI/input"
	rs "github.com/mohamadkrayem/requestCLI/response"
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

// SendOptions carries the per-invocation transport and output settings.
type SendOptions struct {
	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	Redirect    bool
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

// GenerateUrl builds the request URL, applying a default scheme and query params.
//
// Scheme-less URLs default to https, except loopback hosts (localhost,
// 127.0.0.1, [::1]) which default to http so local development keeps working.
// forceHTTP overrides both.
func GenerateUrl(reqURL string, forceHTTP bool, queryParams map[string]string) (string, error) {
	reqURL = strings.TrimSpace(reqURL)
	if reqURL == "" {
		return "", errors.New("no URL given")
	}

	if !hasScheme(reqURL) {
		reqURL = defaultScheme(reqURL, forceHTTP) + "://" + reqURL
	}

	parsed, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", reqURL, err)
	}
	// Hostname() rather than Host: a malformed input such as "://x" parses to
	// the non-empty host ":" with no actual hostname in it.
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid URL %q: no host", reqURL)
	}

	if len(queryParams) > 0 {
		query := parsed.Query()
		for key, value := range queryParams {
			query.Set(key, value)
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

// hasScheme checks for an explicit http/https prefix.
//
// It must anchor at the start: a URL such as "example.com/?to=http://x" contains
// a scheme without starting with one.
func hasScheme(reqURL string) bool {
	lower := strings.ToLower(reqURL)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func defaultScheme(reqURL string, forceHTTP bool) string {
	if forceHTTP || isLoopback(reqURL) {
		return "http"
	}
	return "https"
}

// isLoopback reports whether a scheme-less URL points at the local machine.
func isLoopback(reqURL string) bool {
	host := reqURL
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")

	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// AddQueryString merges query parameters into the request URL.
//
// It parses the existing URL rather than appending "?", so params added here
// combine correctly with any already present.
func (req *BaseRequest) AddQueryString(queryParams map[string]any) error {
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", req.URL, err)
	}

	query := parsed.Query()
	for key, value := range queryParams {
		query.Set(key, toQueryValue(value))
	}
	parsed.RawQuery = query.Encode()

	req.URL = parsed.String()
	return nil
}

// toQueryValue renders a decoded JSON value as a query-string value.
func toQueryValue(value any) string {
	switch m := value.(type) {
	case string:
		return m
	case bool:
		return strconv.FormatBool(m)
	case int:
		return strconv.Itoa(m)
	case float32:
		return strconv.FormatFloat(float64(m), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(m, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", m)
	}
}

// GenerateQueryParams encodes either a map[string]string or a map[string]any.
func GenerateQueryParams(data any) string {
	query := url.Values{}
	switch m := data.(type) {
	case map[string]string:
		for key, value := range m {
			query.Set(key, value)
		}
	case map[string]any:
		for key, value := range m {
			query.Set(key, toQueryValue(value))
		}
	}
	return query.Encode()
}

// sendsBodyInQuery reports whether a method carries its data as query
// parameters rather than as a request body.
func sendsBodyInQuery(method string) bool {
	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodHead,
		http.MethodTrace, http.MethodOptions, http.MethodConnect:
		return true
	}
	return false
}

// WithBody attaches the body to the request according to the form/multipart flags.
func (req *BaseRequest) WithBody(body string, form, multipartForm bool) error {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}

	switch {
	case form:
		mapBody, err := formats.ToMapOptionalJS(body)
		if err != nil {
			return fmt.Errorf("--form needs a json object as the body: %w", err)
		}
		if sendsBodyInQuery(req.Method) {
			return req.AddQueryString(mapBody)
		}
		req.Body = GenerateQueryParams(mapBody)
		req.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		return nil

	case multipartForm:
		multipartInput, err := input.NewMultipartInputInJSONFormat(body)
		if err != nil {
			return err
		}
		req.MultipartBody = multipartInput.Body
		req.Writer = multipartInput.Writer
		req.Headers["Content-Type"] = multipartInput.Writer.FormDataContentType()
		return nil

	default:
		if sendsBodyInQuery(req.Method) {
			mapBody, err := formats.ToMapOptionalJS(body)
			if err != nil {
				// Not JSON, so it cannot become query params: send it verbatim.
				req.Body = body
				req.Headers["Content-Type"] = "text/plain"
				return nil
			}
			return req.AddQueryString(mapBody)
		}
		req.Body = body
		return nil
	}
}

// Send sends the request and returns the rendered response.
func (req *BaseRequest) Send(opts SendOptions) (*rs.Response, error) {
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

	resp, err := client.Do(reqHttp)
	if err != nil {
		return nil, fmt.Errorf("sending %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	newRes, err := rs.NewResponse(resp, opts.ShowStatus, opts.ShowHeaders, opts.ShowBody)
	if err != nil {
		return nil, err
	}
	return newRes, nil
}

// addDefaultHeaders fills in headers the user did not set explicitly.
func (req *BaseRequest) addDefaultHeaders(hasBody bool) {
	if req.Headers == nil {
		req.Headers = make(map[string]any)
	}
	setIfAbsent(req.Headers, "Accept", "*/*")
	setIfAbsent(req.Headers, "User-Agent", "requestCLI/"+Version)
	// Requested explicitly because the response layer decodes brotli itself,
	// which net/http does not do.
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
