/*
Copyright © 2023 Mohamad Krayem <mohamadkrayem@email.com>
*/
package cmd

import (
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

var put_command command.Command = command.Command{
	&BodyJS,
	"PUT",
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

// putCmd represents the put command
var putCmd = &cobra.Command{
	Use:   "put",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		put_command.Run(args, cmd)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		put_command.Method = "PUT"
		put_command.Body = Body
		put_command.Headers = Headers
		put_command.Form = &Form
		put_command.Sb = ShowBody
		put_command.Sh = ShowHeaders
		put_command.Ss = ShowStatus
		put_command.Https = Https
		put_command.BodyJS = &BodyJS
		put_command.HeadersJS = &HeadersJS
		put_command.Headersjs = &Headersjs
		put_command.PersistentPreRun(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(putCmd)
}
