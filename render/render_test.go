package render

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mohamadkrayem/requestCLI/core"
)

// result builds a Result without going near the network.
func result(contentType, body string) *core.Result {
	headers := http.Header{}
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	headers.Set("X-Marker", "here")
	return &core.Result{
		Proto:      "HTTP/1.1",
		Status:     "200 OK",
		StatusCode: 200,
		Headers:    headers,
		Body:       []byte(body),
	}
}

func TestRenderShowsEverythingWhenNoFlagsGiven(t *testing.T) {
	got := Render(result("text/plain", "payload"), Options{})

	for _, want := range []string{"200 OK", "X-Marker", "payload"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q is missing %q", got, want)
		}
	}
}

// The three print flags are a selection set, so they combine.
func TestRenderCombinesSelectionFlags(t *testing.T) {
	got := Render(result("text/plain", "payload"), Options{ShowHeaders: true, ShowBody: true})

	if strings.Contains(got, "200 OK") {
		t.Error("status was not requested but is present")
	}
	if !strings.Contains(got, "X-Marker") {
		t.Error("headers were requested but are missing")
	}
	if !strings.Contains(got, "payload") {
		t.Error("body was requested but is missing")
	}
}

func TestRenderStatusOnly(t *testing.T) {
	got := Render(result("text/plain", "payload"), Options{ShowStatus: true})

	if !strings.Contains(got, "200 OK") {
		t.Error("status missing")
	}
	if strings.Contains(got, "X-Marker") || strings.Contains(got, "payload") {
		t.Errorf("only the status was requested, got %q", got)
	}
}

// Render must be a pure function of its inputs: the TUI re-renders the same
// Result on every resize, and a snapshot diff is meaningless if two renders of
// one response differ.
func TestRenderIsDeterministic(t *testing.T) {
	r := result("application/json", `{"b":1,"a":2,"c":[1,2,3]}`)
	opts := Options{Color: true}

	first := Render(r, opts)
	for i := 0; i < 20; i++ {
		if got := Render(r, opts); got != first {
			t.Fatalf("render %d differs from the first:\n%q\n%q", i, got, first)
		}
	}
}

func TestColorOffEmitsNoEscapeSequences(t *testing.T) {
	bodies := map[string]string{
		"application/json": `{"a":1,"b":"two"}`,
		"text/html":        "<html><body><p>hi</p></body></html>",
		"application/xml":  "<root><item>1</item></root>",
		"text/plain":       "just text",
	}

	for contentType, body := range bodies {
		got := Render(result(contentType, body), Options{Color: false})
		if strings.Contains(got, "\x1b[") {
			t.Errorf("%s output contains ANSI escapes with Color off: %q", contentType, got)
		}
	}
}

// Regression: colorizeHTML ignored the terminal check that the JSON path
// applied, so piping HTML output produced raw escape sequences.
func TestColorOnEmitsEscapeSequencesForEveryType(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/html", "application/xml"} {
		body := `{"a":1}`
		if contentType != "application/json" {
			body = "<root><item>1</item></root>"
		}
		got := Render(result(contentType, body), Options{ShowBody: true, Color: true})
		if !strings.Contains(got, "\x1b[") {
			t.Errorf("%s output has no colour with Color on: %q", contentType, got)
		}
	}
}

// Regression: numbers were decoded into float64 for display, so any integer
// beyond float64's 53-bit mantissa was silently corrupted.
func TestLargeIntegersSurviveRendering(t *testing.T) {
	const id = "1234567890123456789"

	got := Render(result("application/json", `{"id":`+id+`}`), Options{ShowBody: true})
	if !strings.Contains(got, id) {
		t.Errorf("output %q lost the exact integer %s", got, id)
	}
}

// Regression: json.Marshal on a map sorts keys, so the body displayed was never
// the body received.
func TestKeyOrderIsPreserved(t *testing.T) {
	got := Render(result("application/json", `{"zebra":1,"apple":2,"mango":3}`), Options{ShowBody: true})

	zebra := strings.Index(got, "zebra")
	apple := strings.Index(got, "apple")
	mango := strings.Index(got, "mango")
	if zebra < 0 || apple < 0 || mango < 0 {
		t.Fatalf("output is missing keys: %q", got)
	}
	if zebra >= apple || apple >= mango {
		t.Errorf("keys were reordered; got %q", got)
	}
}

// Regression: removeNewLines stripped \n out of string *values*, corrupting any
// description or markdown field.
func TestNewlinesInsideStringValuesSurvive(t *testing.T) {
	got := Render(result("application/json", `{"note":"line1\nline2"}`), Options{ShowBody: true})

	if !strings.Contains(got, `line1\nline2`) {
		t.Errorf("output %q lost the escaped newline inside the string value", got)
	}
}

// Duplicate keys used to collapse silently when the payload was decoded.
func TestDuplicateKeysAreShown(t *testing.T) {
	got := Render(result("application/json", `{"a":1,"a":2}`), Options{ShowBody: true})

	if strings.Count(got, `"a"`) != 2 {
		t.Errorf("output %q collapsed the duplicate key", got)
	}
}

// A Content-Type of application/json with an unparseable payload should still
// render, rather than failing the whole command.
func TestInvalidJSONFallsBackToRaw(t *testing.T) {
	got := Render(result("application/json", "this is not json"), Options{ShowBody: true})

	if !strings.Contains(got, "this is not json") {
		t.Errorf("output = %q, want the raw payload", got)
	}
}

func TestEmptyBodyRendersNothing(t *testing.T) {
	if got := Render(result("application/json", ""), Options{ShowBody: true}); got != "" {
		t.Errorf("output = %q, want empty", got)
	}
}

func TestRepeatedHeadersAreAllRendered(t *testing.T) {
	r := result("text/plain", "")
	r.Headers.Add("X-Multi", "one")
	r.Headers.Add("X-Multi", "two")

	got := Render(r, Options{ShowHeaders: true})
	for _, want := range []string{"one", "two"} {
		if !strings.Contains(got, want) {
			t.Errorf("headers %q missing value %q", got, want)
		}
	}
}

func TestRenderHandlesNilResult(t *testing.T) {
	if got := Render(nil, Options{}); got != "" {
		t.Errorf("Render(nil) = %q, want empty", got)
	}
}
