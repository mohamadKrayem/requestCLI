/*
Copyright © 2023 Mohamad Krayem mohamadkrayem@email.com
*/
package cmd

import (
	//	"bufio"
	//	"os"
	//	"strings"

	"github.com/mohamadkrayem/requestCLI/command"
	//	json "github.com/mohamadkrayem/requestCLI/formats"
	//	rq "github.com/mohamadkrayem/requestCLI/requests"
	"github.com/spf13/cobra"
	//"strings"
)

// getCmd represents the get command
var GetCmd = &cobra.Command{
	Use:   "get",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		command.Run(args, cmd, &bodyJS, "GET", &headersjs, &headersJS, https)
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		command.PersistentPreRun(cmd, args, body, headers, &bodyJS, &headersJS, headersjs)
	},
	Args: cobra.MinimumNArgs(1),
}

func init() {
	rootCmd.AddCommand(GetCmd)
}