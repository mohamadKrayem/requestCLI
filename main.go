/*
Copyright © 2023 Mohamad Krayem < mohamadkrayem@email.com >
*/

// Command requestCLI is the pre-rename entry point, kept for one release so
// `go install github.com/mohamadkrayem/requestCLI@latest` keeps working. That
// installs a binary named requestCLI, which prints a deprecation notice on
// every run. The canonical entry point is ./cmd/rq.
package main

import "github.com/mohamadkrayem/requestCLI/cmd"

func main() {
	cmd.Execute()
}
