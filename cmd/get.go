/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dimasma0305/ctfify/function/creds"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/scraper/ctfd"

	"github.com/hokaccha/go-prettyjson"
	"github.com/spf13/cobra"
)

type getCmdFlags struct {
	creds          creds.CredsStruct
	verbose        bool
	filterCategory string
	onlySolved     bool
}

var getFlag getCmdFlags

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Download ctfd challenges from url",
	Run: func(cmd *cobra.Command, args []string) {
		if err := getFlag.creds.Validate(); err != nil {
			log.Fatal(err)
		}
		ctf, err := ctfd.Init(getFlag.creds.Url, &ctfd.Creds{
			Username: getFlag.creds.Username,
			Password: getFlag.creds.Password,
		})
		if err != nil {
			log.Fatal(err)
		}
		challenges, err := ctf.GetChallenges()
		if err != nil {
			log.Fatal(err)
		}

		// filter category
		if cmd.Flags().Lookup("filter-category").Changed {
			challenges = challenges.Filter(func(chall *ctfd.ChallengeInfo) bool {
				return strings.EqualFold(chall.Category, getFlag.filterCategory)
			})
		}

		// filter solved
		if getFlag.onlySolved {
			challenges = challenges.Filter(func(chall *ctfd.ChallengeInfo) bool {
				return chall.Solved_By_Me
			})
		}

		var wg sync.WaitGroup
		for _, chall := range challenges {
			wg.Add(1)
			go func(challenge *ctfd.ChallengeInfo) {
				defer wg.Done()
				dstFolder := filepath.Join(ctf.HostName(), challenge.Category, challenge.Name)
				fullInfo, err := challenge.GetFullInfo()
				if err != nil {
					log.Fatal(err)
				}
				if err := fullInfo.WriteTemplatesToDirDefault(dstFolder); err != nil {
					log.Fatal(err)
				}
				if err := fullInfo.DownloadFilesToDir(filepath.Join(dstFolder, "attachment")); err != nil {
					log.Fatal(err)
				}
				log.Info("success downloading: %s (%s)", challenge.Name, challenge.Category)
				if getFlag.verbose {
					data, _ := prettyjson.Marshal(fullInfo)
					fmt.Println(string(data))
				}
			}(chall)
		}
		wg.Wait()
	},
}

func init() {
	rootCmd.AddCommand(getCmd)

	getCmd.Flags().StringVarP(&getFlag.creds.Url, "url", "u", "", "CTF platform url to get")
	getCmd.Flags().StringVarP(&getFlag.creds.Username, "username", "s", "", "Username")
	getCmd.Flags().StringVarP(&getFlag.creds.Password, "password", "p", "", "Password")
	getCmd.Flags().StringVarP(&getFlag.filterCategory, "filter-category", "c", "", "Filter challenge by category")
	getCmd.Flags().BoolVarP(&getFlag.onlySolved, "only-solved", "o", false, "Filter challenge by category")
	getCmd.Flags().BoolVarP(&getFlag.verbose, "verbose", "v", false, "Make the log more verbose")

}
