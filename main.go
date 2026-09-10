/*
Copyright © 2023 Mohamad Krayem < mohamadkrayem@email.com >
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mohamadkrayem/requestCLI/cmd"
)

func main() {
	// requestCLI is the old binary name, kept working as a symlink for one
	// release. Warn on stderr, then continue exactly as before.
	switch filepath.Base(os.Args[0]) {
	case "requestCLI", "requestCLI.exe":
		fmt.Fprintln(os.Stderr, "requestCLI is deprecated and will be removed in the next release; use rq")
	}
	cmd.Execute()
}
