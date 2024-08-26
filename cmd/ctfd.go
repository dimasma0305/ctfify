/*
Copyright © 2023 dimas maulana dimasmaulana0305@gmail.com
*/
package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/scraper/ctfd"

	"github.com/hokaccha/go-prettyjson"
	"github.com/spf13/cobra"
)

// ctfdCmd represents the get command
var ctfdCmd = &cobra.Command{
	Use:   "ctfd",
	Short: "Download ctfd challenges from url",
	Run: func(cmd *cobra.Command, args []string) {
		type Creds struct {
			username string
			password string
			url      string
		}
		var (
			creds = &Creds{
				username: cmd.Flag("username").Value.String(),
				password: cmd.Flag("password").Value.String(),
				url:      cmd.Flag("url").Value.String(),
			}
		)

		ctf, err := ctfd.Init(creds.url, &ctfd.Creds{
			Username: creds.username,
			Password: creds.password,
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
				var filterCategory = cmd.Flag("filter-category").Value.String()
				return strings.EqualFold(chall.Category, filterCategory)
			})
		}

		// filter solved
		if onlySolved, _ := cmd.Flags().GetBool("only-solved"); onlySolved {
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
				log.SuccessDownload(challenge.Name, challenge.Category)
				if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
					data, _ := prettyjson.Marshal(fullInfo)
					fmt.Println(string(data))
				}
			}(chall)
		}
		wg.Wait()
	},
}

func init() {
	rootCmd.AddCommand(ctfdCmd)

	ctfdCmd.Flags().StringP("url", "u", "", "CTF platform url to get")
	ctfdCmd.Flags().StringP("username", "s", "", "Username")
	ctfdCmd.Flags().StringP("password", "p", "", "Password")
	ctfdCmd.Flags().StringP("filter-category", "c", "", "Filter challenge by category")
	ctfdCmd.Flags().BoolP("only-solved", "o", false, "Filter challenge by category")
	ctfdCmd.Flags().BoolP("verbose", "v", false, "Make the log more verbose")

}
