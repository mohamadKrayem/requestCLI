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

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "requestCLI",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
	rootCmd.PersistentFlags().BoolVarP(&Https, "secure", "s", false, "https or http.")
	rootCmd.PersistentFlags().BoolVarP(&Form, "form", "f", false, "Send a form.")
	rootCmd.PersistentFlags().StringToStringVarP(&QueryParams, "query", "q", nil, "Write your query params.")
	rootCmd.PersistentFlags().StringToStringVarP(&Cookies, "cookie", "c", nil, "Write your cookies.")
	rootCmd.PersistentFlags().StringToStringVarP(&Auth, "auth", "a", nil, "Write your basic auth.")
	rootCmd.PersistentFlags().BoolVar(&Body, "body", false, "Write your nested json.")
	rootCmd.PersistentFlags().BoolVar(&Headers, "headers", false, "Write your nested headers in json format.")
	rootCmd.PersistentFlags().StringVarP(&BodyJS, "Nbody", "b", "", "Write your simple body.")
	rootCmd.PersistentFlags().StringToStringVarP(&HeadersJS, "Nheaders", "n", nil, "Write your simple headers.")
	rootCmd.PersistentFlags().BoolVarP(&ShowBody, "printB", "B", false, "Print only the body.")
	rootCmd.PersistentFlags().BoolVarP(&ShowHeaders, "printH", "H", false, "Print only the headers.")
	rootCmd.PersistentFlags().BoolVarP(&ShowStatus, "printS", "S", false, "Print only the status.")
	rootCmd.PersistentFlags().BoolVarP(&Text, "text", "t", false, "Send plain text")                             // to be implemented
	rootCmd.PersistentFlags().BoolVarP(&ReqHeaders, "reqHeaders", "r", false, "Print only the request headers.") // to be implemented

}
