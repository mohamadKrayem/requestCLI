/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	"os"

	json "github.com/mohamadkrayem/requestCLI/formats"
	"github.com/spf13/cobra"
)

var (
	QueryParams map[string]string
	Cookies     map[string]string
	Auth        map[string]string
	Body        bool
	BodyJS      string
	Headers     bool
	HeadersJS   map[string]string
	Https       bool
	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	Form        bool
	Text        bool
	ReqHeaders  bool
	Headersjs   json.Json
)

type Cmd struct {
	Method      string
	QueryParams map[string]string
	Cookies     map[string]string
	Auth        map[string]string
	Body        bool
	BodyJS      string
	Headers     bool
	HeadersJS   map[string]string
	Https       bool
	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	Form        bool
	Text        bool
	ReqHeaders  bool
	Headersjs   json.Json
	Redirect    bool
}

var Command = Cmd{}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "requestCLI",
	Short: "RequestCLI is a CLI tool that allows you to send HTTP requests to a server.",
	Long: `RequestCLI is a CLI tool that allows you to send HTTP requests to a server. 
It is a simple tool that allows you to send requests with different methods,
headers, cookies, query params, body, and authentication.
It also allows you to print the response in different formats.
It deals with various data compression algorithms such as deflated, gzip, and br.
	`,
	/*Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("----------------------------------------------------------------")
		fmt.Println(args)
		fmt.Println("----------------------------------------------------------------")
	},*/
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.requestCLI.yaml)")

	rootCmd.Flags().Bool("toggle", false, "Help message for toggle.")
	rootCmd.PersistentFlags().BoolVarP(&Command.Https, "secure", "s", false, "Send a secure request.")
	rootCmd.PersistentFlags().BoolVarP(&Command.Form, "form", "f", false, "Send a form.")
	rootCmd.PersistentFlags().StringToStringVarP(&Command.QueryParams, "query", "q", nil, "Write your query params.")
	rootCmd.PersistentFlags().StringToStringVarP(&Command.Cookies, "cookie", "c", nil, "Set your cookies.")
	rootCmd.PersistentFlags().StringToStringVarP(&Command.Auth, "auth", "a", nil, "Set your basic-auth.")
	rootCmd.PersistentFlags().BoolVar(&Command.Body, "body", false, "Write your nested body in json format.")
	rootCmd.PersistentFlags().BoolVar(&Command.Headers, "headers", false, "Write your nested headers in json format.")
	rootCmd.PersistentFlags().StringVarP(&Command.BodyJS, "Nbody", "b", "", "Write your body in a simple json format on a single line.")
	rootCmd.PersistentFlags().StringToStringVarP(&Command.HeadersJS, "Nheaders", "n", nil, "Write your headers in a simple json format on a single line.")
	rootCmd.PersistentFlags().BoolVarP(&Command.ShowBody, "printB", "B", false, "Print only the body of the response.")
	rootCmd.PersistentFlags().BoolVarP(&Command.ShowHeaders, "printH", "H", false, "Print only the headers of the response.")
	rootCmd.PersistentFlags().BoolVarP(&Command.ShowStatus, "printS", "S", false, "Print only the status code of the response.")
	rootCmd.PersistentFlags().BoolVarP(&Command.Text, "text", "t", false, "Send plain text")                             // to be implemented
	rootCmd.PersistentFlags().BoolVarP(&Command.ReqHeaders, "reqHeaders", "r", false, "Print only the request headers.") // to be implemented
	rootCmd.PersistentFlags().BoolVar(&Command.Redirect, "Redirect", false, "Follow Redirects")
}
