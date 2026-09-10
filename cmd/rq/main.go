// Command rq is a terminal HTTP client with HTTPie-style request items.
//
// This is the canonical entry point. It exists as its own directory because
// `go install` names a binary after the last element of its import path:
//
//	go install github.com/mohamadkrayem/requestCLI/cmd/rq@latest
//
// installs rq, where installing the module root installs requestCLI.
package main

import "github.com/mohamadkrayem/requestCLI/cmd"

func main() {
	cmd.Execute()
}
