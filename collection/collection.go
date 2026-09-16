// Package collection discovers the directory a set of .http files lives in and
// assembles the variable scopes that apply to them.
//
// A collection is a directory containing an rq.toml, found by walking up from
// the request file the way git finds .git. There is no manifest listing every
// request, no export step and no ids: adding a request means creating a file
// and renaming one is mv. That is the whole reason to be file-based rather
// than reimplementing a GUI client's model in a text editor.
//
// It imports httpfile for the Source interface and nothing else beyond the
// standard library and a TOML decoder. It does not import core: configuration
// is not a transport concern.
package collection

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mohamadkrayem/requestCLI/httpfile"
)

const (
	// ConfigName marks a directory as a collection, and is also the file a
	// nested directory uses to override what it inherits.
	ConfigName = "rq.toml"
	// EnvironmentsDir holds the committed, non-secret environment files.
	EnvironmentsDir = "environments"
	// SecretsName is gitignored and never committed.
	SecretsName = ".rq.secrets.toml"
)

// Defaults are the settings a collection applies to every request beneath it.
//
// This is the one thing genuinely worth taking from a GUI client: set the base
// URL and the auth once, then write requests that do not repeat themselves.
type Defaults struct {
	Headers map[string]string
	Auth    Auth
}

// Auth is a collection-level credential.
type Auth struct {
	Type  string `toml:"type"`
	Token string `toml:"token"`
	// Username and Password are used when Type is "basic".
	Username string `toml:"username"`
	Password string `toml:"password"`
}

// Config is one rq.toml.
type Config struct {
	// Path is the file this came from, used as a variable origin.
	Path string
	// Vars are the scalar keys under [defaults].
	Vars map[string]string
	// Defaults are the [defaults.headers] and [defaults.auth] tables.
	Defaults Defaults
}

// Collection is a discovered directory tree.
type Collection struct {
	// Root is the directory holding the outermost rq.toml.
	Root string
	// Configs run from the root inward, so a nested rq.toml merges over its
	// parent simply by coming later.
	Configs []Config
}

// ErrNotFound reports that no rq.toml was found walking up from a directory.
// It is not a failure on its own: a lone .http file with no collection around
// it is a perfectly ordinary thing to run.
var ErrNotFound = errors.New("no rq.toml found")

// Discover walks up from dir looking for rq.toml, and returns the chain of
// them from the outermost inward.
//
// The outermost is the root because a nested config is the more specific
// statement and must win, which it does by being applied later.
func Discover(dir string) (*Collection, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var found []string
	for current := abs; ; {
		candidate := filepath.Join(current, ConfigName)
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("looking for %s: %w", candidate, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if len(found) == 0 {
		return nil, ErrNotFound
	}

	// found is innermost-first from the walk; reverse it so the root leads.
	slicesReverse(found)

	c := &Collection{Root: filepath.Dir(found[0])}
	for _, path := range found {
		config, err := LoadConfig(path)
		if err != nil {
			return nil, err
		}
		c.Configs = append(c.Configs, config)
	}
	return c, nil
}

// LoadConfig reads one rq.toml.
func LoadConfig(path string) (Config, error) {
	var raw struct {
		Defaults map[string]any `toml:"defaults"`
	}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	config := Config{Path: path, Vars: map[string]string{}}

	for key, value := range raw.Defaults {
		switch key {
		case "headers":
			headers, err := stringMap(value)
			if err != nil {
				return Config{}, fmt.Errorf("%s: [defaults.headers]: %w", path, err)
			}
			config.Defaults.Headers = headers

		case "auth":
			auth, err := decodeAuth(value)
			if err != nil {
				return Config{}, fmt.Errorf("%s: [defaults.auth]: %w", path, err)
			}
			config.Defaults.Auth = auth

		default:
			// Any other scalar under [defaults] is a variable. Keeping them in
			// the same table as headers and auth is what the format looks like
			// in practice, so the parser sorts them out rather than the user.
			text, ok := scalar(value)
			if !ok {
				return Config{}, fmt.Errorf("%s: [defaults] %s must be a string, number or boolean", path, key)
			}
			config.Vars[key] = text
		}
	}
	return config, nil
}

// Environment loads environments/<name>.toml relative to the collection root.
func (c *Collection) Environment(name string) (httpfile.MapSource, error) {
	path := filepath.Join(c.Root, EnvironmentsDir, name+".toml")

	values, err := LoadFlat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return httpfile.MapSource{}, fmt.Errorf("no environment %q: %s does not exist%s", name, path, c.availableEnvironments())
		}
		return httpfile.MapSource{}, err
	}
	return httpfile.MapSource{Values: values, Label: path}, nil
}

// availableEnvironments lists what the collection does define, so a typo is a
// one-step fix rather than a hunt.
func (c *Collection) availableEnvironments() string {
	entries, err := os.ReadDir(filepath.Join(c.Root, EnvironmentsDir))
	if err != nil {
		return ""
	}

	var names []string
	for _, entry := range entries {
		if name, ok := strings.CutSuffix(entry.Name(), ".toml"); ok && !entry.IsDir() {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return "; available: " + strings.Join(names, ", ")
}

// Secrets loads .rq.secrets.toml from the collection root.
//
// A missing file is not an error: secrets may come from the environment
// instead, which is the CI escape hatch.
func (c *Collection) Secrets() (map[string]string, string, error) {
	path := filepath.Join(c.Root, SecretsName)

	values, err := LoadFlat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", nil
		}
		return nil, "", err
	}
	return values, path, nil
}

// Vars returns one Source per config, root first, so nested configs override
// their parents by position rather than by merging maps.
func (c *Collection) Vars() []httpfile.Source {
	sources := make([]httpfile.Source, 0, len(c.Configs))
	for _, config := range c.Configs {
		sources = append(sources, httpfile.MapSource{Values: config.Vars, Label: config.Path})
	}
	return sources
}

// Defaults merges the headers and auth of every config, innermost winning.
func (c *Collection) Defaults() Defaults {
	merged := Defaults{Headers: map[string]string{}}

	for _, config := range c.Configs {
		for name, value := range config.Defaults.Headers {
			merged.Headers[name] = value
		}
		// Auth is replaced wholesale rather than field-merged: a nested
		// collection that switches from bearer to basic must not inherit half
		// of what it replaced.
		if config.Defaults.Auth.Type != "" {
			merged.Auth = config.Defaults.Auth
		}
	}
	return merged
}

// LoadFlat reads a TOML file of top-level key/value pairs. It is exported for
// the system-wide variable file, which lives outside any collection.
func LoadFlat(path string) (map[string]string, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	values := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := scalar(value)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be a string, number or boolean", path, key)
		}
		values[key] = text
	}
	return values, nil
}

// scalar renders a TOML scalar as the string a request will carry. Numbers and
// booleans are accepted because writing port = 8080 unquoted is natural and
// rejecting it would be pedantry.
func scalar(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case int64:
		return fmt.Sprintf("%d", v), true
	case float64:
		return fmt.Sprintf("%v", v), true
	case bool:
		return fmt.Sprintf("%t", v), true
	default:
		return "", false
	}
}

func stringMap(value any) (map[string]string, error) {
	table, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("expected a table")
	}

	out := make(map[string]string, len(table))
	for key, raw := range table {
		text, ok := scalar(raw)
		if !ok {
			return nil, fmt.Errorf("%s must be a string, number or boolean", key)
		}
		out[key] = text
	}
	return out, nil
}

func decodeAuth(value any) (Auth, error) {
	table, ok := value.(map[string]any)
	if !ok {
		return Auth{}, errors.New("expected a table")
	}

	auth := Auth{}
	for key, raw := range table {
		text, ok := scalar(raw)
		if !ok {
			return Auth{}, fmt.Errorf("%s must be a string", key)
		}
		switch key {
		case "type":
			auth.Type = text
		case "token":
			auth.Token = text
		case "username":
			auth.Username = text
		case "password":
			auth.Password = text
		default:
			return Auth{}, fmt.Errorf("unknown key %q; expected type, token, username or password", key)
		}
	}
	return auth, nil
}

func slicesReverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
