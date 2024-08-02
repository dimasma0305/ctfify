/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

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
		var gz *gzcli.GZ
		var err error
		if init, _ := cmd.Flags().GetBool("init"); init {
			if err := gz.InitFolder(); err != nil {
				log.Fatal(err)
			}
			return
		}

		if sync, _ := cmd.Flags().GetBool("sync"); sync {
			if gz, err = gzcli.Init(); err != nil {
				log.Fatal(err)
			}
			if err := gz.Sync(); err != nil {
				log.Fatal(err)
			}
			return
		}

		if ctftime, _ := cmd.Flags().GetBool("ctftime-scoreboard"); ctftime {
			if gz, err = gzcli.Init(); err != nil {
				log.Fatal(err)
			}
			feed, err := gz.Scoreboard2CTFTimeFeed()
			if err != nil {
				log.Fatal(err)
			}
			b, err := json.Marshal(feed)
			if err != nil {
				log.Fatal(err)
			}
			var out bytes.Buffer
			err = json.Indent(&out, b, "", "  ")
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(out.String())
			return
		}

		if script, _ := cmd.Flags().GetString("run-script"); script != "" {
			if err := gz.RunScript(script); err != nil {
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
	gzcliCmd.Flags().Bool("ctftime-scoreboard", false, "generate ctftime scoreboard feed")
	gzcliCmd.Flags().String("run-script", "", "run script")
}
