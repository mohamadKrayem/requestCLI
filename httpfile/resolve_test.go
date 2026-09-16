package httpfile

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	if got.Request.URL != "https://example.com/users" {
		t.Errorf("URL = %q", got.Request.URL)
	}
	if got.Request.Headers[0].Value != "Bearer abc" {
		t.Errorf("header = %q", got.Request.Headers[0].Value)
	}
	if string(got.Request.Body) != `{"name":"ada"}` {
		t.Errorf("Body = %q", got.Request.Body)
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
	if string(got.Request.Body) != `{"from":"file"}` {
		t.Errorf("Body = %q", got.Request.Body)
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

// A namespaced reference resolves only from its namespace, never from the
// ordinary chain. This is the property that stops a secret silently shadowing
// an ordinary variable — the failure mode the whole design exists to prevent.
func TestNamespacesAreNotPrecedenceLevels(t *testing.T) {
	r := Resolver{
		Sources: []Source{MapSource{Values: map[string]string{"token": "ordinary"}, Label: "file"}},
		Namespaces: map[string]Source{
			SecretNamespace: MapSource{Values: map[string]string{"token": "s3cret"}, Label: ".rq.secrets.toml"},
		},
	}

	ordinary, ok := r.Lookup("token")
	if !ok || ordinary.Value != "ordinary" {
		t.Errorf("{{token}} = %+v, want the ordinary value", ordinary)
	}
	if ordinary.Secret {
		t.Error("an ordinary variable was marked secret")
	}

	secret, ok := r.Lookup("secret:token")
	if !ok || secret.Value != "s3cret" {
		t.Errorf("{{secret:token}} = %+v, want the secret value", secret)
	}
	if !secret.Secret {
		t.Error("a secret was not marked as one")
	}
}

// An unknown namespace is not silently treated as an ordinary variable.
func TestUnknownNamespaceIsNotResolved(t *testing.T) {
	r := Resolver{Sources: []Source{MapSource{Values: map[string]string{"nope:x": "leaked"}, Label: "file"}}}

	if _, ok := r.Lookup("nope:x"); ok {
		t.Error("a namespaced reference matched an ordinary variable")
	}
}

func TestEnvNamespaceReadsTheProcessEnvironment(t *testing.T) {
	t.Setenv("RQ_TEST_ENV_VAR", "from-env")
	r := Resolver{Namespaces: map[string]Source{EnvNamespace: EnvSource{}}}

	got, err := r.Expand("{{env:RQ_TEST_ENV_VAR}}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got != "from-env" {
		t.Errorf("Expand = %q", got)
	}
}

func TestDynamicVariablesAreGenerated(t *testing.T) {
	r := Resolver{}

	uuid, err := r.Expand("{{$uuid}}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(uuid) != 36 || strings.Count(uuid, "-") != 4 {
		t.Errorf("$uuid = %q, want a canonical uuid", uuid)
	}
	// Version 4, variant 10.
	if uuid[14] != '4' {
		t.Errorf("$uuid = %q, want version 4", uuid)
	}
	if !strings.ContainsRune("89ab", rune(uuid[19])) {
		t.Errorf("$uuid = %q, want the RFC variant bits", uuid)
	}

	timestamp, err := r.Expand("{{$timestamp}}", 1)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		t.Errorf("$timestamp = %q, want an integer", timestamp)
	}

	if _, err := r.Expand("{{$randomInt}}", 1); err != nil {
		t.Errorf("$randomInt: %v", err)
	}
}

// Two references to the same built-in in one request must agree. A correlation
// id that differed between a header and the body it labels would be worse than
// useless.
func TestDynamicVariablesAreStableWithinARequest(t *testing.T) {
	r := Resolver{}
	request := Request{
		URL:     "https://example.com/{{$uuid}}",
		Headers: []Header{{Name: "X-Request-Id", Value: "{{$uuid}}"}},
	}

	got, err := r.Resolve(request, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	fromURL := strings.TrimPrefix(got.Request.URL, "https://example.com/")
	if fromURL != got.Request.Headers[0].Value {
		t.Errorf("two {{$uuid}} in one request disagreed: %q and %q", fromURL, got.Request.Headers[0].Value)
	}
}

// ...but differ between requests, or they would not be identifiers.
func TestDynamicVariablesDifferBetweenRequests(t *testing.T) {
	r := Resolver{}
	request := Request{URL: "{{$uuid}}"}

	first, _ := r.Resolve(request, "", "")
	second, _ := r.Resolve(request, "", "")

	if first.Request.URL == second.Request.URL {
		t.Error("two requests got the same {{$uuid}}")
	}
}

func TestUnknownDynamicVariableIsReported(t *testing.T) {
	r := Resolver{}

	if _, err := r.Expand("{{$nonsense}}", 3); err == nil {
		t.Fatal("expected an error for an unknown built-in")
	}
}

// Resolve reports the secret values it substituted, so output can mask them.
func TestResolveReportsSubstitutedSecrets(t *testing.T) {
	r := Resolver{
		Namespaces: map[string]Source{
			SecretNamespace: MapSource{Values: map[string]string{"token": "s3cret-value"}, Label: "secrets"},
		},
	}
	request := Request{
		URL:     "https://example.com/",
		Headers: []Header{{Name: "Authorization", Value: "Bearer {{secret:token}}"}},
	}

	got, err := r.Resolve(request, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Secrets) != 1 || got.Secrets[0] != "s3cret-value" {
		t.Errorf("Secrets = %v, want the substituted value", got.Secrets)
	}
}

// A committed request resolving from a machine-local scope is a
// reproducibility footgun: it works here and fails for a teammate. Resolve
// reports it so the caller can warn.
func TestResolveReportsSystemScopedVariables(t *testing.T) {
	r := Resolver{Sources: []Source{
		MapSource{Values: map[string]string{"host": "localhost"}, Label: "~/.config/rq/vars.toml", System: true},
	}}

	got, err := r.Resolve(Request{URL: "http://{{host}}/"}, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.SystemScoped) != 1 || got.SystemScoped[0] != "host" {
		t.Errorf("SystemScoped = %v, want [host]", got.SystemScoped)
	}
}

// A variable that came from a committed scope must not be reported as
// system-scoped, or the warning becomes noise people ignore.
func TestNonSystemVariablesAreNotFlagged(t *testing.T) {
	r := Resolver{Sources: []Source{
		MapSource{Values: map[string]string{"host": "example.com"}, Label: "rq.toml"},
	}}

	got, err := r.Resolve(Request{URL: "http://{{host}}/"}, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.SystemScoped != nil {
		t.Errorf("SystemScoped = %v, want none", got.SystemScoped)
	}
}
