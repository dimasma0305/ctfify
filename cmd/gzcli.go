/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/dimasma0305/ctfify/function/gzcli"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/spf13/cobra"
)

// gzcliCmd represents the gzcli command
var gzcliCmd = &cobra.Command{
	Use:   "gzcli",
	Short: "gzcli is a command line interface for gz::ctf",
	Long:  `gzcli is a command line interface for gz::ctf`,
	Run: func(cmd *cobra.Command, args []string) {
		if init, _ := cmd.Flags().GetBool("init"); init {
			if err := gzcli.InitFolder(); err != nil {
				log.Fatal(err)
			}
			return
		}

		if sync, _ := cmd.Flags().GetBool("sync"); sync {
			if err := gzcli.Sync(); err != nil {
				log.Fatal(err)
			}
			return
		}
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(gzcliCmd)
	gzcliCmd.Flags().Bool("init", false, "init gzcli")
	gzcliCmd.Flags().Bool("sync", false, "update gzcli")

}
