/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	auth "github.com/mohamadkrayem/requestCLI/authentication"
	command "github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

// command.Command is a struct that contains all the data entered by the user in a single command
// it can be found in command/parentCMD.go
var trace_command command.Command = command.NewCommand()

// TraceCmd represents the trace command
var TraceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Send a TRACE request to a server.",
	Run: func(cmd *cobra.Command, args []string) {
		trace_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		trace_command.Method = "CONNECT"
		trace_command.Body = Command.Body
		trace_command.Headers = Command.Headers
		trace_command.Form = &Command.Form
		trace_command.Sb = Command.ShowBody
		trace_command.Sh = Command.ShowHeaders
		trace_command.Ss = Command.ShowStatus
		trace_command.QueryParams = &Command.QueryParams
		trace_command.Https = Command.Https
		trace_command.BodyJS = &Command.BodyJS
		trace_command.HeadersJS = &Command.HeadersJS
		trace_command.Headersjs = &Command.Headersjs
		trace_command.Redirect = Command.Redirect
		trace_command.Cookies = &Command.Cookies
		trace_command.Multipart = Command.Multipart
		trace_command.BasicAuth = auth.NewBaseRequestFromMap(Command.Auth)
		trace_command.PersistentPreRun(cmd, args)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(TraceCmd)
}
