package cmd

import (
	"net/http"
	"testing"
)

// Regression: the trace subcommand was wired to CONNECT by a copy-paste slip.
func TestEachSubcommandSendsItsOwnVerb(t *testing.T) {
	want := map[string]string{
		"get":     http.MethodGet,
		"post":    http.MethodPost,
		"put":     http.MethodPut,
		"patch":   http.MethodPatch,
		"del":     http.MethodDelete,
		"head":    http.MethodHead,
		"options": http.MethodOptions,
		"trace":   http.MethodTrace,
		"connect": http.MethodConnect,
	}

	if len(methods) != len(want) {
		t.Fatalf("methods table has %d entries, want %d", len(methods), len(want))
	}

	for _, m := range methods {
		expected, ok := want[m.use]
		if !ok {
			t.Errorf("unexpected subcommand %q", m.use)
			continue
		}
		if m.method != expected {
			t.Errorf("subcommand %q sends %q, want %q", m.use, m.method, expected)
		}
	}
}

// Regression: head, options, put and patch were missing their Args validator
// and panicked on args[0] when no URL was given.
func TestEverySubcommandRequiresExactlyOneURL(t *testing.T) {
	for _, m := range methods {
		t.Run(m.use, func(t *testing.T) {
			c := newMethodCmd(m.use, m.method, m.short, m.aliases)

			if c.Args == nil {
				t.Fatal("subcommand has no argument validator")
			}
			if err := c.Args(c, []string{}); err == nil {
				t.Error("accepted zero arguments; it should require a URL")
			}
			if err := c.Args(c, []string{"example.com"}); err != nil {
				t.Errorf("rejected a single URL: %v", err)
			}
			if err := c.Args(c, []string{"a.com", "b.com"}); err == nil {
				t.Error("accepted two URLs; it should take exactly one")
			}
		})
	}
}

// The README documents `delete`, while the command is registered as `del`.
func TestDeleteIsAcceptedAsAnAlias(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"delete"})
	if err != nil {
		t.Fatalf("finding the delete alias: %v", err)
	}
	if c.Name() != "del" {
		t.Errorf("`delete` resolved to %q, want del", c.Name())
	}
}

func TestConnIsAcceptedAsAnAlias(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"conn"})
	if err != nil {
		t.Fatalf("finding the conn alias: %v", err)
	}
	if c.Name() != "connect" {
		t.Errorf("`conn` resolved to %q, want connect", c.Name())
	}
}

func TestSecurityRelevantFlagsExist(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	for _, name := range []string{"insecure", "http", "timeout", "redirect"} {
		if flags.Lookup(name) == nil {
			t.Errorf("missing --%s flag", name)
		}
	}

	// TLS verification must be opt-out, never the default.
	if got := flags.Lookup("insecure").DefValue; got != "false" {
		t.Errorf("--insecure defaults to %q, want false", got)
	}
	// An unbounded default timeout is what let a hung server hang the CLI.
	if got := flags.Lookup("timeout").DefValue; got == "0s" {
		t.Error("--timeout defaults to 0s, which means no timeout")
	}
}
