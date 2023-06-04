/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var post_command command.Command = command.NewCommand()

// postCmd represents the post command
var postCmd = &cobra.Command{
	Use:   "post",
	Short: "Send a POST request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		post_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		post_command.Method = "POST"
		post_command.Body = Command.Body
		post_command.Headers = Command.Headers
		post_command.Form = &Command.Form
		post_command.Sb = Command.ShowBody
		post_command.Sh = Command.ShowHeaders
		post_command.Ss = Command.ShowStatus
		post_command.Https = Command.Https
		post_command.BodyJS = &Command.BodyJS
		post_command.HeadersJS = &Command.HeadersJS
		post_command.Headersjs = &Command.Headersjs
		post_command.Redirect = Command.Redirect
		post_command.Cookies = &Command.Cookies
		post_command.Multipart = Command.Multipart
		post_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		post_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(postCmd)
}
