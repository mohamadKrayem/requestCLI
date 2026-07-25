package render

import (
	"os"
	"testing"
)

// A pipe is not a terminal, so colour must be off regardless of environment.
func TestColorDisabledWhenNotATerminal(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	os.Unsetenv("NO_COLOR")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if ColorEnabled(w) {
		t.Error("colour enabled for a pipe; piped output would carry ANSI escapes")
	}
}

func TestColorDisabledByNoColor(t *testing.T) {
	// https://no-color.org: the variable being present is enough, whatever
	// its value.
	for _, value := range []string{"", "1", "false"} {
		t.Setenv("NO_COLOR", value)
		if ColorEnabled(os.Stdout) {
			t.Errorf("NO_COLOR=%q did not disable colour", value)
		}
	}
}

func TestColorDisabledForDumbTerminal(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	t.Setenv("TERM", "dumb")

	if ColorEnabled(os.Stdout) {
		t.Error("TERM=dumb did not disable colour")
	}
}

func TestColorDisabledForNilFile(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	t.Setenv("TERM", "xterm-256color")

	if ColorEnabled(nil) {
		t.Error("a nil file should never be treated as a terminal")
	}
}
