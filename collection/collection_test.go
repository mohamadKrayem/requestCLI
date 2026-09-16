package collection

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree writes a set of files under a temp dir and returns its root.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()

	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Discovery walks up the way git finds .git, so a request file anywhere in the
// tree finds the collection above it.
func TestDiscoverWalksUp(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":               "[defaults]\nbase_url = \"https://example.com\"\n",
		"users/admin/list.http": "GET {{base_url}}/\n",
	})

	c, err := Discover(filepath.Join(root, "users", "admin"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if c.Root != root {
		t.Errorf("Root = %q, want %q", c.Root, root)
	}
	if len(c.Configs) != 1 {
		t.Fatalf("got %d configs, want 1", len(c.Configs))
	}
}

// A lone .http file with no collection is ordinary, not an error.
func TestDiscoverReportsNotFound(t *testing.T) {
	_, err := Discover(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Discover = %v, want ErrNotFound", err)
	}
}

// Configs run root-first so a nested one wins by being applied later.
func TestDiscoverOrdersConfigsRootFirst(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":       "[defaults]\nscope = \"root\"\n",
		"users/rq.toml": "[defaults]\nscope = \"nested\"\n",
	})

	c, err := Discover(filepath.Join(root, "users"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(c.Configs) != 2 {
		t.Fatalf("got %d configs, want 2", len(c.Configs))
	}
	if c.Configs[0].Vars["scope"] != "root" {
		t.Errorf("Configs[0] = %v, want the root first", c.Configs[0].Vars)
	}
	if c.Configs[1].Vars["scope"] != "nested" {
		t.Errorf("Configs[1] = %v, want the nested one second", c.Configs[1].Vars)
	}
	// The root of the collection is where the outermost config lives.
	if c.Root != root {
		t.Errorf("Root = %q, want %q", c.Root, root)
	}
}

// [defaults] mixes variables with the headers and auth tables, which is what
// the format looks like in practice.
func TestLoadConfigSeparatesVarsFromTables(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml": `[defaults]
base_url = "https://example.com"
port = 8443
verbose = true

[defaults.headers]
Accept = "application/json"

[defaults.auth]
type  = "bearer"
token = "{{secret:api_token}}"
`,
	})

	config, err := LoadConfig(filepath.Join(root, "rq.toml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if config.Vars["base_url"] != "https://example.com" {
		t.Errorf("base_url = %q", config.Vars["base_url"])
	}
	// Numbers and booleans are accepted unquoted: writing port = 8443 is
	// natural and rejecting it would be pedantry.
	if config.Vars["port"] != "8443" {
		t.Errorf("port = %q, want 8443", config.Vars["port"])
	}
	if config.Vars["verbose"] != "true" {
		t.Errorf("verbose = %q, want true", config.Vars["verbose"])
	}
	if config.Vars["headers"] != "" || config.Vars["auth"] != "" {
		t.Error("headers/auth leaked into the variables")
	}
	if config.Defaults.Headers["Accept"] != "application/json" {
		t.Errorf("headers = %v", config.Defaults.Headers)
	}
	if config.Defaults.Auth.Type != "bearer" || config.Defaults.Auth.Token != "{{secret:api_token}}" {
		t.Errorf("auth = %+v", config.Defaults.Auth)
	}
}

func TestLoadConfigRejectsAnUnknownAuthKey(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml": "[defaults.auth]\ntyp = \"bearer\"\n",
	})

	_, err := LoadConfig(filepath.Join(root, "rq.toml"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "typ") {
		t.Errorf("error = %q, want it to name the key", err)
	}
}

// Headers merge across the chain; auth is replaced wholesale so a nested
// collection switching from bearer to basic does not inherit half of what it
// replaced.
func TestDefaultsMergeHeadersButReplaceAuth(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml": `[defaults.headers]
Accept = "application/json"
X-Root = "root"

[defaults.auth]
type  = "bearer"
token = "root-token"
`,
		"users/rq.toml": `[defaults.headers]
X-Nested = "nested"

[defaults.auth]
type     = "basic"
username = "someone"
`,
	})

	c, err := Discover(filepath.Join(root, "users"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	defaults := c.Defaults()

	for name, want := range map[string]string{
		"Accept":   "application/json",
		"X-Root":   "root",
		"X-Nested": "nested",
	} {
		if defaults.Headers[name] != want {
			t.Errorf("header %s = %q, want %q", name, defaults.Headers[name], want)
		}
	}

	if defaults.Auth.Type != "basic" {
		t.Errorf("auth type = %q, want basic", defaults.Auth.Type)
	}
	if defaults.Auth.Token != "" {
		t.Errorf("auth token = %q, want the bearer token not to survive a switch to basic", defaults.Auth.Token)
	}
}

// A nested header overrides rather than duplicating.
func TestDefaultsNestedHeaderWins(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":       "[defaults.headers]\nAccept = \"application/json\"\n",
		"users/rq.toml": "[defaults.headers]\nAccept = \"text/plain\"\n",
	})

	c, _ := Discover(filepath.Join(root, "users"))
	if got := c.Defaults().Headers["Accept"]; got != "text/plain" {
		t.Errorf("Accept = %q, want the nested value", got)
	}
}

func TestEnvironmentLoads(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":                   "[defaults]\nbase_url = \"https://prod\"\n",
		"environments/staging.toml": "base_url = \"https://staging\"\n",
	})

	c, _ := Discover(root)
	source, err := c.Environment("staging")
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	if value, _ := source.Lookup("base_url"); value != "https://staging" {
		t.Errorf("base_url = %q", value)
	}
}

// A typo should be a one-step fix, not a hunt.
func TestEnvironmentListsWhatExists(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":                   "[defaults]\n",
		"environments/dev.toml":     "a = \"1\"\n",
		"environments/staging.toml": "a = \"2\"\n",
	})

	c, _ := Discover(root)
	_, err := c.Environment("prod")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"dev", "staging"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to list %q", err, want)
		}
	}
}

func TestSecretsLoad(t *testing.T) {
	root := tree(t, map[string]string{
		"rq.toml":          "[defaults]\n",
		".rq.secrets.toml": "api_token = \"s3cret\"\n",
	})

	c, _ := Discover(root)
	secrets, path, err := c.Secrets()
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if secrets["api_token"] != "s3cret" {
		t.Errorf("secrets = %v", secrets)
	}
	if !strings.HasSuffix(path, SecretsName) {
		t.Errorf("path = %q", path)
	}
}

// A missing secrets file is not an error: secrets may come from the
// environment instead, which is the CI escape hatch.
func TestSecretsAreOptional(t *testing.T) {
	root := tree(t, map[string]string{"rq.toml": "[defaults]\n"})

	c, _ := Discover(root)
	secrets, _, err := c.Secrets()
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("secrets = %v, want none", secrets)
	}
}

func TestLoadFlatRejectsANestedTable(t *testing.T) {
	root := tree(t, map[string]string{
		"environments/bad.toml": "[nested]\nkey = \"value\"\n",
	})

	_, err := LoadFlat(filepath.Join(root, "environments", "bad.toml"))
	if err == nil {
		t.Fatal("expected an error for a nested table in a flat file")
	}
}
