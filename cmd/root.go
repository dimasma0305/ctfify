/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ctfify",
	Short: "Tools for downloading CTF challenges from various platforms.",
	Long: `ctfify is a command-line tool designed to simplify the process of downloading and managing Capture The Flag (CTF) challenges.
With ctfify, you can easily search for CTF challenges by name, category, or tag, and download them directly to your local machine with just a few commands.`,
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
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
