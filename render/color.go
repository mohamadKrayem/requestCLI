package render

import (
	"os"

	"github.com/mattn/go-isatty"
)

// ColorEnabled decides once, at the boundary, whether output should be coloured.
//
// It is deliberately separate from Render: Render takes the answer as a plain
// bool so it stays pure and testable. Previously each renderer decided for
// itself, so JSON was gated on the terminal check and HTML was not — piping
// HTML output into grep produced raw escape sequences.
//
// NO_COLOR is honoured per https://no-color.org: any value, including empty,
// disables colour.
func ColorEnabled(f *os.File) bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if f == nil {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
