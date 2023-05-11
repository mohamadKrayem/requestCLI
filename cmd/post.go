/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var post_command command.Command = command.Command{
	&BodyJS,
	"POST",
	&Headersjs,
	&HeadersJS,
	Https,
	ShowStatus,
	ShowHeaders,
	ShowBody,
	&Form,
	Body,
	Headers,
}

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		post_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		post_command.Method = "PUT"
		post_command.Body = Body
		post_command.Headers = Headers
		post_command.Form = &Form
		post_command.Sb = ShowBody
		post_command.Sh = ShowHeaders
		post_command.Ss = ShowStatus
		post_command.Https = Https
		post_command.BodyJS = &BodyJS
		post_command.HeadersJS = &HeadersJS
		post_command.Headersjs = &Headersjs
		post_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(postCmd)
}
