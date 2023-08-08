/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
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
			events, err := ctftime.Init().GetEventsByDate(1000000, start, finish)
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
			if cmd.Flags().Changed("description") {
				var description = cmd.Flag("description").Value.String()
				events = events.Filter(func(chall *ctftime.Event) bool {
					return strings.Contains(strings.ToLower(chall.Description), strings.ToLower(description))
				})
			}
			if cmd.Flags().Changed("days") {
				var days, err = cmd.Flags().GetIntSlice("days")
				if err != nil {
					log.Fatal(err)
				}
				events = events.Filter(func(chall *ctftime.Event) bool {
					ctfDaysName, err := chall.EventDays()
					if err != nil {
						log.Fatal(err)
					}
					for _, ctfDay := range ctfDaysName {
						for _, day := range days {
							if day == ctfDay {
								return true
							}
						}
					}
					return false
				})
			}
			if cmd.Flags().Changed("print-keys") {
				keys, err := cmd.Flags().GetStringSlice("print-keys")
				if err != nil {
					log.Fatal(err)
				}
				var newEventFormat []map[string]interface{}
				for _, event := range events {
					mapevent, err := structToMap(event)
					if err != nil {
						log.Fatal(err)
					}
					newEvent := make(map[string]interface{})
					for _, key := range keys {
						newEvent[key] = mapevent[key]
					}
					newEventFormat = append(newEventFormat, newEvent)
				}
				data, _ := prettyjson.Marshal(newEventFormat)
				fmt.Println(string(data))
			} else {
				data, _ := prettyjson.Marshal(events)
				fmt.Println(string(data))
			}
		},
	}
	evetsCmd.Flags().String("start", "", "date start of ctf (example: 'may 2, 2023')")
	evetsCmd.Flags().String("finish", "", "data finish of ctf (example: 'december 20, 2023')")
	evetsCmd.Flags().String("organizer", "", "filter ctf by organizer name")
	evetsCmd.Flags().String("title", "", "filter ctf by title")
	evetsCmd.Flags().String("description", "", "filter ctf by description")
	evetsCmd.Flags().IntSlice("days", []int{}, "filter ctf by day (example: '1,2,3,4,5,6,7')")
	evetsCmd.Flags().StringSlice("print-keys", []string{}, "print the value of a specific keys in the event map")
	ctftimeCmd.AddCommand(evetsCmd)
}

// make struct to map
func structToMap(obj interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
