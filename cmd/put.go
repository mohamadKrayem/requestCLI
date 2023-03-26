/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/mohamadkrayem/requestCLI/command"
	"github.com/spf13/cobra"
)

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
		command.Run(args, cmd, &bodyJS, "PUT", &headersjs, &headersJS, https)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		command.PersistentPreRun(cmd, args, body, headers, &bodyJS, &headersJS, headersjs)
	},
}

func init() {
	rootCmd.AddCommand(putCmd)
}
