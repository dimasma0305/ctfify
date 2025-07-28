/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"os"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ctfify",
	Short: "Tools for downloading CTF challenges from various platforms.",
	Long: `ctfify is a command-line tool designed to simplify the process of downloading and managing Capture The Flag (CTF) challenges.
With ctfify, you can easily search for CTF challenges by name, category, or tag, and download them directly to your local machine with just a few commands.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Enable debug mode if flag is set
		if debug, _ := cmd.Flags().GetBool("debug"); debug {
			log.SetDebugMode(true)
			log.Debug("Debug mode enabled")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Add debug flag to root command
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logging")
}
