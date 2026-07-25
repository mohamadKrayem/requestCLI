package core

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

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
