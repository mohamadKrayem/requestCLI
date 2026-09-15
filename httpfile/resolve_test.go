package httpfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resolverWith(pairs ...map[string]string) Resolver {
	var sources []Source
	for i, values := range pairs {
		sources = append(sources, MapSource{Values: values, Label: labelFor(i)})
	}
	return Resolver{Sources: sources}
}

func labelFor(i int) string {
	return []string{"file", "--var", "extra"}[i]
}

func TestExpandSubstitutesPlaceholders(t *testing.T) {
	r := resolverWith(map[string]string{"base": "https://example.com", "id": "42"})

	got, err := r.Expand("{{base}}/users/{{id}}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got != "https://example.com/users/42" {
		t.Errorf("Expand = %q", got)
	}
}

func TestExpandToleratesInnerWhitespace(t *testing.T) {
	r := resolverWith(map[string]string{"base": "x"})

	got, err := r.Expand("{{ base }}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got != "x" {
		t.Errorf("Expand = %q, want x", got)
	}
}

// Later sources win, so the slice reads lowest precedence first.
func TestLookupPrefersTheLastSource(t *testing.T) {
	r := resolverWith(
		map[string]string{"host": "from-file"},
		map[string]string{"host": "from-flag"},
	)

	resolution, ok := r.Lookup("host")
	if !ok {
		t.Fatal("Lookup returned false")
	}
	if resolution.Value != "from-flag" {
		t.Errorf("Value = %q, want from-flag", resolution.Value)
	}
	if resolution.Origin != "--var" {
		t.Errorf("Origin = %q, want --var", resolution.Origin)
	}
}

// An unresolved variable must be named, not left in the URL to fail later as
// a confusing connection error.
func TestExpandReportsAnUnknownVariable(t *testing.T) {
	r := resolverWith(map[string]string{})

	_, err := r.Expand("{{base}}/users", 7)
	if err == nil {
		t.Fatal("expected an error")
	}
	var unknown *UnknownVariableError
	if !errors.As(err, &unknown) {
		t.Fatalf("error is %T, want *UnknownVariableError", err)
	}
	if unknown.Name != "base" {
		t.Errorf("Name = %q, want base", unknown.Name)
	}
	if unknown.Line != 7 {
		t.Errorf("Line = %d, want 7", unknown.Line)
	}
}

// A namespaced form the full model will support must produce a clear
// "unknown variable" rather than a parse failure.
func TestExpandNamesNamespacedVariables(t *testing.T) {
	r := resolverWith(map[string]string{})

	_, err := r.Expand("{{secret:api_token}}", 1)
	var unknown *UnknownVariableError
	if !errors.As(err, &unknown) {
		t.Fatalf("error is %T, want *UnknownVariableError", err)
	}
	if unknown.Name != "secret:api_token" {
		t.Errorf("Name = %q", unknown.Name)
	}
}

// Substitution is single-pass: a value containing {{...}} is not re-expanded.
func TestExpandDoesNotRecurse(t *testing.T) {
	r := resolverWith(map[string]string{"a": "{{b}}", "b": "final"})

	got, err := r.Expand("{{a}}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got != "{{b}}" {
		t.Errorf("Expand = %q, want the value left alone", got)
	}
}

func TestResolveSubstitutesThroughoutARequest(t *testing.T) {
	r := resolverWith(map[string]string{"base": "https://example.com", "token": "abc", "who": "ada"})
	request := Request{
		Method:  "POST",
		URL:     "{{base}}/users",
		Headers: []Header{{Name: "X-Token", Value: "Bearer {{token}}"}},
		Body:    []byte(`{"name":"{{who}}"}`),
		Line:    3,
	}

	got, err := r.Resolve(request, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.URL != "https://example.com/users" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.Headers[0].Value != "Bearer abc" {
		t.Errorf("header = %q", got.Headers[0].Value)
	}
	if string(got.Body) != `{"name":"ada"}` {
		t.Errorf("Body = %q", got.Body)
	}
}

// Resolve must not mutate the parsed request, so a file can be resolved twice
// against different variables.
func TestResolveLeavesTheOriginalUntouched(t *testing.T) {
	r := resolverWith(map[string]string{"base": "https://example.com"})
	request := Request{
		URL:     "{{base}}/a",
		Headers: []Header{{Name: "H", Value: "{{base}}"}},
	}

	if _, err := r.Resolve(request, "", ""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if request.URL != "{{base}}/a" {
		t.Errorf("original URL was mutated: %q", request.URL)
	}
	if request.Headers[0].Value != "{{base}}" {
		t.Errorf("original header was mutated: %q", request.Headers[0].Value)
	}
}

func TestResolveReadsABodyFileRelativeToTheDocument(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "payload.json"), []byte(`{"from":"file"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	r := resolverWith(map[string]string{})
	got, err := r.Resolve(Request{BodyFile: "./payload.json"}, dir, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(got.Body) != `{"from":"file"}` {
		t.Errorf("Body = %q", got.Body)
	}
}

func TestResolveReportsAMissingBodyFile(t *testing.T) {
	r := resolverWith(map[string]string{})

	_, err := r.Resolve(Request{BodyFile: "./absent.json", Line: 4}, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Errorf("error = %q, want it to name the line", err)
	}
}

func TestFileVarsTakesTheLastDefinition(t *testing.T) {
	file := &File{Vars: []Var{
		{Name: "host", Value: "first"},
		{Name: "host", Value: "second"},
	}}

	source := FileVars(file, "api.http")
	if value, _ := source.Lookup("host"); value != "second" {
		t.Errorf("Lookup = %q, want second", value)
	}
}

func TestNamesListsPlaceholdersInOrder(t *testing.T) {
	request := Request{
		URL:     "{{base}}/users/{{id}}",
		Headers: []Header{{Name: "X", Value: "{{token}}"}},
		Body:    []byte("{{base}}"),
	}

	got := Names(request)
	want := []string{"base", "id", "token"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseVarFlag(t *testing.T) {
	name, value, err := ParseVarFlag("host=example.com")
	if err != nil {
		t.Fatalf("ParseVarFlag: %v", err)
	}
	if name != "host" || value != "example.com" {
		t.Errorf("got %q=%q", name, value)
	}

	// An empty value is legitimate: it clears an inherited variable.
	if _, value, err := ParseVarFlag("host="); err != nil || value != "" {
		t.Errorf("ParseVarFlag(host=) = %q, %v", value, err)
	}

	for _, bad := range []string{"novalue", "=orphan", ""} {
		if _, _, err := ParseVarFlag(bad); err == nil {
			t.Errorf("ParseVarFlag(%q) should have failed", bad)
		}
	}
}
