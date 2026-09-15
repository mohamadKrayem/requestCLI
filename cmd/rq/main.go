// Command rq is a terminal HTTP client with HTTPie-style request items.
//
// It exists as its own directory because `go install` names a binary after the
// last element of its import path, and the module is still named requestCLI:
//
//	go install github.com/mohamadkrayem/requestCLI/cmd/rq@latest
//
// installs a binary called rq.
package main

import "github.com/mohamadkrayem/requestCLI/cmd"

func main() {
	cmd.Execute()
}
