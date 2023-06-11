/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var put_command command.Command = command.NewCommand()

// putCmd represents the put command
var putCmd = &cobra.Command{
	Use:   "put",
	Short: "Send a PUT request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		put_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		put_command.Method = "PUT"
		put_command.Body = Command.Body
		put_command.Headers = Command.Headers
		put_command.Form = &Command.Form
		put_command.Sb = Command.ShowBody
		put_command.Sh = Command.ShowHeaders
		put_command.Ss = Command.ShowStatus
		put_command.Https = Command.Https
		put_command.BodyJS = &Command.BodyJS
		put_command.HeadersJS = &Command.HeadersJS
		put_command.Headersjs = &Command.Headersjs
		put_command.Redirect = Command.Redirect
		put_command.QueryParams = &Command.QueryParams
		put_command.Cookies = &Command.Cookies
		put_command.Multipart = Command.Multipart
		put_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		put_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(putCmd)
}
