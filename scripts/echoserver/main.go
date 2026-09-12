// Command echoserver is a local fixture for exercising rq by hand.
//
// It echoes back whatever it received and exposes endpoints for redirects,
// compression, auth, slow responses and arbitrary status codes. It serves both
// plain HTTP and HTTPS (with a self-signed certificate) so the TLS behaviour of
// the client can be tested without touching the network.
//
//	go run ./scripts/echoserver
//
// See TESTING.md for the scenarios that use it.
package main

import (
	"compress/flate"
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	httpAddr := flag.String("http", "127.0.0.1:8080", "plain HTTP listen address")
	tlsAddr := flag.String("https", "127.0.0.1:8443", "HTTPS listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", echo)
	mux.HandleFunc("/json", serveJSON)
	mux.HandleFunc("/html", serveHTML)
	mux.HandleFunc("/text", serveText)
	mux.HandleFunc("/gzip", compressed("gzip"))
	mux.HandleFunc("/deflate", compressed("deflate"))
	mux.HandleFunc("/redirect", redirect)
	mux.HandleFunc("/moved", moved)
	mux.HandleFunc("/slow", slow)
	mux.HandleFunc("/sse", serveSSE)
	mux.HandleFunc("/status/", status)
	mux.HandleFunc("/basic-auth", basicAuth)
	mux.HandleFunc("/multipart", multipartEcho)
	mux.HandleFunc("/fidelity", serveFidelity)
	mux.HandleFunc("/binary", serveBinary)
	mux.HandleFunc("/xml", serveXML)
	mux.HandleFunc("/yaml", serveYAML)

	cert, err := selfSignedCert()
	if err != nil {
		log.Fatalf("generating certificate: %v", err)
	}

	go func() {
		srv := &http.Server{
			Addr:              *tlsAddr,
			Handler:           mux,
			TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}},
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Printf("https listening on https://%s (self-signed)", *tlsAddr)
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("https server: %v", err)
		}
	}()

	srv := &http.Server{
		Addr:              *httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("http  listening on http://%s", *httpAddr)
	log.Fatal(srv.ListenAndServe())
}

// echoed is the JSON shape returned by the catch-all handler.
type echoed struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query"`
	Headers map[string]string `json:"headers"`
	Cookies map[string]string `json:"cookies"`
	Body    string            `json:"body"`
	TLS     bool              `json:"tls"`
}

func echo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	out := echoed{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   map[string]string{},
		Headers: map[string]string{},
		Cookies: map[string]string{},
		Body:    string(body),
		TLS:     r.TLS != nil,
	}
	for k, v := range r.URL.Query() {
		out.Query[k] = strings.Join(v, ",")
	}
	for k, v := range r.Header {
		out.Headers[k] = strings.Join(v, ",")
	}
	for _, c := range r.Cookies() {
		out.Cookies[c.Name] = c.Value
	}

	// Log a one-line summary so the terminal running the server is readable.
	log.Printf("%s %s body=%q", r.Method, r.URL.RequestURI(), truncate(string(body), 120))

	writeJSON(w, http.StatusOK, out)
}

func serveJSON(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "Mohamad",
		"age":     22,
		"active":  true,
		"missing": nil,
		"tags":    []string{"a", "b"},
		"nested":  map[string]any{"deep": map[string]any{"deeper": 1}},
	})
}

// serveFidelity returns a body written as raw bytes rather than marshalled, so
// it can carry the things a decode/re-encode round trip destroys: an integer
// too large for float64, deliberately unsorted keys, a newline inside a string
// value, and a duplicate key.
func serveFidelity(w http.ResponseWriter, _ *http.Request) {
	const body = `{"zebra":1,"id":1234567890123456789,"apple":2,` +
		`"note":"line1\nline2","dup":1,"dup":2}`

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// serveBinary returns a PNG header followed by NUL bytes: printing it raw
// leaves a terminal in a broken state.
func serveBinary(w http.ResponseWriter, _ *http.Request) {
	body := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 2048)...)

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func serveXML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<?xml version="1.0"?><catalog><book id="1">highlighted</book></catalog>`))
}

func serveYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("name: Mohamad\ntags:\n  - a\n  - b\nhighlighted: true\n"))
}

func serveHTML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, "<!doctype html>\n<html>\n<head><title>fixture</title></head>\n"+
		"<body><h1>Hello</h1><p class=\"x\">world</p></body>\n</html>\n")
}

func serveText(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, "plain text response")
}

// compressed returns a handler that encodes its payload with the given scheme.
func compressed(encoding string) http.HandlerFunc {
	payload := []byte(`{"encoding":"` + encoding + `","message":"decoded correctly"}`)

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", encoding)

		var zw io.WriteCloser
		switch encoding {
		case "gzip":
			zw = gzip.NewWriter(w)
		case "deflate":
			fw, err := flate.NewWriter(w, flate.DefaultCompression)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			zw = fw
		default:
			http.Error(w, "unsupported encoding", http.StatusInternalServerError)
			return
		}
		// Close flushes the compressor: a failure here truncates the payload.
		defer func() {
			if err := zw.Close(); err != nil {
				log.Printf("closing %s writer: %v", encoding, err)
			}
		}()

		if _, err := zw.Write(payload); err != nil {
			log.Printf("writing %s payload: %v", encoding, err)
		}
	}
}

func redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/moved", http.StatusFound)
}

func moved(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"followed": "yes"})
}

// slow sleeps before responding, for exercising --timeout.
func slow(w http.ResponseWriter, r *http.Request) {
	seconds := 5
	if v := r.URL.Query().Get("seconds"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			seconds = n
		}
	}
	time.Sleep(time.Duration(seconds) * time.Second)
	writeJSON(w, http.StatusOK, map[string]string{"slept": strconv.Itoa(seconds)})
}

// status returns the code named in the path, e.g. /status/404.
func status(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/status/"))
	if err != nil || code < 100 || code > 599 {
		http.Error(w, "usage: /status/<code>", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"status":%d}`, code)
}

func basicAuth(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no credentials"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"authenticated": "yes",
		"username":      user,
		"password":      pass,
	})
}

// multipartEcho parses a multipart form and reports the fields and files it got.
func multipartEcho(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	fields := map[string]string{}
	for k, v := range r.MultipartForm.Value {
		fields[k] = strings.Join(v, ",")
	}

	files := map[string]string{}
	names := make([]string, 0, len(r.MultipartForm.File))
	for k := range r.MultipartForm.File {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fh := r.MultipartForm.File[k][0]
		files[k] = fmt.Sprintf("%s (%d bytes)", fh.Filename, fh.Size)
	}

	log.Printf("multipart fields=%v files=%v", fields, files)
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields, "files": files})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		log.Printf("writing json: %v", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// selfSignedCert builds an in-memory certificate valid for localhost, so the
// HTTPS listener fails verification unless the client passes --insecure.
func selfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"requestCLI test fixture"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// serveSSE streams server-sent events.
//
// Query parameters:
//
//	events=N     how many events to send (default 3, capped at 100)
//	delay=Nms    pause between events (default 10ms, capped at 5s)
//	keepalive=1  send a comment frame before the events, which a client must
//	             not render as an empty event
//	noterm=1     omit the blank line after the final event, so a client can be
//	             checked against a stream that is cut short
func serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	count := 3
	if n, err := strconv.Atoi(r.URL.Query().Get("events")); err == nil && n >= 0 {
		count = min(n, 100)
	}

	delay := 10 * time.Millisecond
	if d, err := time.ParseDuration(r.URL.Query().Get("delay")); err == nil && d >= 0 {
		delay = min(d, 5*time.Second)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	if r.URL.Query().Get("keepalive") == "1" {
		_, _ = io.WriteString(w, ": keepalive\n\n")
		flusher.Flush()
	}

	for i := range count {
		// A JSON payload, because that is what a real event stream carries and
		// it exercises the renderer's compaction path.
		data := fmt.Sprintf(`{"index":%d,"text":"chunk %d"}`, i, i)
		frame := fmt.Sprintf("event: delta\ndata: %s\n", data)

		last := i == count-1
		if !last || r.URL.Query().Get("noterm") != "1" {
			frame += "\n"
		}

		if _, err := io.WriteString(w, frame); err != nil {
			return
		}
		flusher.Flush()

		if !last {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
	}
}
