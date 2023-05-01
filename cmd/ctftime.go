/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/dimasma0305/ctfify/function/ctftime"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/hokaccha/go-prettyjson"
	"github.com/spf13/cobra"
)

// ctftimeCmd represents the ctftime command

func init() {
	ctftimeCmd := &cobra.Command{
		Use:   "ctftime",
		Short: "A brief description of your command",
	}
	rootCmd.AddCommand(ctftimeCmd)
	evetsCmd := &cobra.Command{
		Use:   "events",
		Short: "get events",
		Run: func(cmd *cobra.Command, args []string) {
			if !cmd.Flags().Changed("start") || !cmd.Flags().Changed("finish") {
				log.Fatal(fmt.Errorf("start and finish must be present"))
			}
			var (
				start  = cmd.Flag("start").Value.String()
				finish = cmd.Flag("finish").Value.String()
			)
			events, err := ctftime.Init().GetEventsByDate(10000, start, finish)
			if err != nil {
				log.Fatal(err)
			}
			// filter by organizer
			if cmd.Flags().Changed("organizer") {
				var organizer = cmd.Flag("organizer").Value.String()
				events = events.Filter(func(chall *ctftime.Event) bool {
					for _, orgname := range chall.Organizers {
						if strings.Contains(strings.ToLower(orgname.Name), strings.ToLower(organizer)) {
							return true
						}
					}
					return false
				})
			}
			// filter by title aka ctf name
			if cmd.Flags().Changed("title") {
				var title = cmd.Flag("title").Value.String()
				events = events.Filter(func(chall *ctftime.Event) bool {
					return strings.Contains(strings.ToLower(chall.Title), strings.ToLower(title))
				})
			}
			data, _ := prettyjson.Marshal(events)
			fmt.Println(string(data))
		},
	}
	evetsCmd.Flags().String("start", "", "date start of ctf (example: 'may 2, 2023')")
	evetsCmd.Flags().String("finish", "", "data finish of ctf (example: 'december 20, 2023')")
	evetsCmd.Flags().String("organizer", "", "filter ctf by organizer name")
	evetsCmd.Flags().String("title", "", "filter ctf by title")
	ctftimeCmd.AddCommand(evetsCmd)
}
