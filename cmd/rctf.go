/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"path/filepath"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/scraper/rctf"
	"github.com/spf13/cobra"
)

// rctfCmd represents the rctf command
var rctfCmd = &cobra.Command{
	Use:   "rctf",
	Short: "Download RCTF challenges from url",
	Run: func(cmd *cobra.Command, args []string) {
		var ctf *rctf.RCTFScraper
		var err error
		if cmd.Flag("url-token").Changed {
			urlToken := cmd.Flag("url-token").Value.String()
			ctf, err = rctf.InitFromUrlToken(urlToken)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			url, err := cmd.Flags().GetString("url")
			if err != nil {
				log.Fatal(err)
			}
			token, err := cmd.Flags().GetString("token")
			if err != nil {
				log.Fatal(err)
			}
			ctf, err = rctf.Init(url, token)
			if err != nil {
				log.Fatal(err)
			}
		}

		challenges, err := ctf.GetChalls()
		if err != nil {
			log.Fatal(err)
		}
		for _, challenge := range challenges.Data {
			dstFolder := filepath.Join(ctf.Url.Hostname(), challenge.Category, challenge.Name)
			if err := challenge.WriteTemplatesToDirDefault(dstFolder); err != nil {
				log.Fatal(err)
			}
			for _, file := range challenge.Files {
				file.DowloadFileToDir(filepath.Join(dstFolder, "attachment"))
			}
			log.SuccessDownload(challenge.Name, challenge.Category)
		}
	},
}

func init() {
	rootCmd.AddCommand(rctfCmd)
	rctfCmd.Flags().StringP("url", "u", "", "url of the rctf platform")
	rctfCmd.Flags().StringP("token", "t", "", "your token")
	rctfCmd.Flags().String("url-token", "", "token url from ctf")
}
