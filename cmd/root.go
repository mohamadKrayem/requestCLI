/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/

// Package cmd wires CLI flags to the request builder.
package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/mohamadkrayem/requestCLI/core"
	"github.com/mohamadkrayem/requestCLI/render"
	"github.com/spf13/cobra"
)

// opts collects every flag value for the invocation.
var opts = &command.Options{}

// Deprecated flags, kept so existing scripts keep working.
var (
	legacySecure   bool
	legacyRedirect bool
)

var rootCmd = &cobra.Command{
	Use:     "rq",
	Version: core.Version,
	Short:   "rq is a CLI tool that allows you to send HTTP requests to a server.",
	Long: `rq is a CLI tool that allows you to send HTTP requests to a server.
It is a simple tool that allows you to send requests with different methods,
headers, cookies, query params, body, and authentication.
It also allows you to print the response in different formats.
It deals with various data compression algorithms such as deflate, gzip, and br.

Scheme-less URLs default to https, except loopback hosts (localhost, 127.0.0.1)
which default to http. Use --http to force plain HTTP.`,

	// Errors are printed once by Execute; a usage dump on a runtime failure is noise.
	SilenceUsage:  true,
	SilenceErrors: true,

	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if legacyRedirect {
			opts.Redirect = true
		}
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *command.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				fmt.Fprintln(os.Stderr, "Error:", exitErr.Err)
			}
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// methods lists every HTTP verb exposed as a subcommand.
var methods = []struct {
	use     string
	method  string
	short   string
	aliases []string
}{
	{"get", http.MethodGet, "Send a GET request to a server.", nil},
	{"post", http.MethodPost, "Send a POST request to a server.", nil},
	{"put", http.MethodPut, "Send a PUT request to a server.", nil},
	{"patch", http.MethodPatch, "Send a PATCH request to a server.", nil},
	{"del", http.MethodDelete, "Send a DELETE request to a server.", []string{"delete"}},
	{"head", http.MethodHead, "Send a HEAD request to a server.", nil},
	{"options", http.MethodOptions, "Send an OPTIONS request to a server.", nil},
	{"trace", http.MethodTrace, "Send a TRACE request to a server.", nil},
	{"connect", http.MethodConnect, "Send a CONNECT request to a server.", []string{"conn"}},
}

// newMethodCmd builds the subcommand for one HTTP method.
//
// Every verb is created here so argument validation and method wiring cannot
// drift between them, which is what previously let `trace` send CONNECT and
// left four subcommands without argument checks.
func newMethodCmd(use, method, short string, aliases []string) *cobra.Command {
	return &cobra.Command{
		Use:     use + " URL [REQUEST_ITEM ...]",
		Aliases: aliases,
		Short:   short,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return command.Run(method, args, opts)
		},
		// The positional arguments are a URL and request items, never files:
		// completion must not offer filenames for either.
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func init() {
	flags := rootCmd.PersistentFlags()

	flags.BoolVar(&opts.HTTP, "http", false, "Send over plain HTTP instead of HTTPS.")
	flags.BoolVarP(&opts.Insecure, "insecure", "k", false, "Skip TLS certificate verification (dangerous).")
	flags.DurationVar(&opts.Timeout, "timeout", core.DefaultTimeout, "Overall request timeout.")

	flags.BoolVarP(&opts.Form, "form", "f", false, "Send a url-encoded form.")
	flags.BoolVar(&opts.Multipart, "multi", false, "Send a multipart form.")

	flags.StringToStringVarP(&opts.QueryParams, "query", "q", nil, "Write your query params.")
	flags.StringToStringVarP(&opts.Cookies, "cookie", "c", nil, "Set your cookies.")
	flags.StringToStringVarP(&opts.Auth, "auth", "a", nil, "Set your basic-auth, e.g. username=me,password=secret.")

	flags.BoolVar(&opts.Body, "body", false, "Read a multi-line json body from stdin, terminated by ';'.")
	flags.BoolVar(&opts.Headers, "headers", false, "Read multi-line json headers from stdin, terminated by ';'.")
	flags.StringVarP(&opts.BodyJS, "Nbody", "b", "", "Write your body as json on a single line.")
	flags.StringToStringVarP(&opts.HeadersJS, "Nheaders", "n", nil, "Write your headers as key=value pairs.")

	flags.BoolVarP(&opts.ShowBody, "printB", "B", false, "Print the body of the response.")
	flags.BoolVarP(&opts.ShowHeaders, "printH", "H", false, "Print the headers of the response.")
	flags.BoolVarP(&opts.ShowStatus, "printS", "S", false, "Print the status line of the response.")
	flags.BoolVarP(&opts.Verbose, "verbose", "v", false, "Show the request that was sent, in addition to the response.")

	flags.BoolVar(&opts.Redirect, "redirect", false, "Follow redirects.")
	flags.StringVar(&opts.Style, "style", render.DefaultStyle, "Syntax highlighting theme for non-json bodies.")

	flags.BoolVar(&opts.CheckStatus, "check-status", false, "Exit with HTTPie's 3/4/5 status codes on a 3xx/4xx/5xx response.")
	flags.BoolVar(&opts.IgnoreStdin, "ignore-stdin", false, "Never read a request body from piped stdin.")

	_ = rootCmd.RegisterFlagCompletionFunc("style", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return render.StyleNames(), cobra.ShellCompDirectiveNoFileComp
	})

	// Deprecated: HTTPS is now the default, so --secure is a no-op.
	flags.BoolVarP(&legacySecure, "secure", "s", false, "Deprecated: HTTPS is the default.")
	_ = flags.MarkDeprecated("secure", "HTTPS is now the default; use --http to force plain HTTP")

	// Deprecated: renamed to the conventional lowercase --redirect.
	flags.BoolVar(&legacyRedirect, "Redirect", false, "Deprecated: use --redirect.")
	_ = flags.MarkDeprecated("Redirect", "use --redirect instead")

	for _, m := range methods {
		rootCmd.AddCommand(newMethodCmd(m.use, m.method, m.short, m.aliases))
	}
}
