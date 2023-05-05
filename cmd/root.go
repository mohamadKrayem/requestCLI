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
	queryParams map[string]string
	cookies     map[string]string
	auth        map[string]string
	body        bool
	bodyJS      string
	headers     bool
	headersJS   map[string]string
	https       bool
	ShowStatus  bool
	ShowHeaders bool
	ShowBody    bool
	form        bool
	text        bool
	reqHeaders  bool
	headersjs   json.Json
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
	rootCmd.PersistentFlags().BoolVarP(&https, "secure", "s", false, "https or http.")
	rootCmd.PersistentFlags().BoolVarP(&form, "form", "f", false, "Send a form.")
	rootCmd.PersistentFlags().StringToStringVarP(&queryParams, "query", "q", nil, "Write your query params.")
	rootCmd.PersistentFlags().StringToStringVarP(&cookies, "cookie", "c", nil, "Write your cookies.")
	rootCmd.PersistentFlags().StringToStringVarP(&auth, "auth", "a", nil, "Write your basic auth.")
	rootCmd.PersistentFlags().BoolVar(&body, "body", false, "Write your nested json.")
	rootCmd.PersistentFlags().BoolVar(&headers, "headers", false, "Write your nested headers in json format.")
	rootCmd.PersistentFlags().StringVarP(&bodyJS, "Nbody", "b", "", "Write your simple body.")
	rootCmd.PersistentFlags().StringToStringVarP(&headersJS, "Nheaders", "n", nil, "Write your simple headers.")
	rootCmd.PersistentFlags().BoolVarP(&ShowBody, "sbody", "B", false, "Print only the body.")
	rootCmd.PersistentFlags().BoolVarP(&ShowHeaders, "sheaders", "H", false, "Print only the headers.")
	rootCmd.PersistentFlags().BoolVarP(&ShowStatus, "status", "S", false, "Print only the status.")
	rootCmd.PersistentFlags().BoolVarP(&text, "text", "t", false, "Send plain text")                             // to be implemented
	rootCmd.PersistentFlags().BoolVarP(&reqHeaders, "reqHeaders", "r", false, "Print only the request headers.") // to be implemented

}
