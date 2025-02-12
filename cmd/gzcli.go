package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dimasma0305/ctfify/function/gzcli"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/dimasma0305/ctfify/function/template/other"
	"github.com/spf13/cobra"
)

type tcommandFlags struct {
	initFlag         bool
	syncFlag         bool
	ctftimeFlag      bool
	scriptFlag       string
	createTeamsFlag  string
	createTeamsEmail string
	deleteUsersFlag  bool
	updateGameFlag   bool
	genStructureFlag bool
}

var commandFlags tcommandFlags

// gzcliCmd represents the optimized gzcli command
var gzcliCmd = &cobra.Command{
	Use:   "gzcli",
	Short: "High-performance CLI for gz::ctf",
	Long:  `Optimized command line interface for gz::ctf operations`,
	Run: func(cmd *cobra.Command, args []string) {
		switch {
		case commandFlags.initFlag:
			other.CTFTemplate(".", map[string]string{})
			return

		case commandFlags.syncFlag:
			gz := gzcli.MustInit()
			gz.UpdateGame = commandFlags.updateGameFlag
			gz.MustSync()

		case commandFlags.ctftimeFlag:
			generateCTFTimeFeed(gzcli.MustInit())

		case commandFlags.scriptFlag != "":
			gzcli.MustRunScripts(commandFlags.scriptFlag)

		case commandFlags.createTeamsFlag != "":
			handleTeamCreation(commandFlags.createTeamsFlag, false)

		case commandFlags.createTeamsEmail != "":
			handleTeamCreation(commandFlags.createTeamsEmail, true)

		case commandFlags.deleteUsersFlag:
			gzcli.MustInit().MustDeleteAllUser()

		case commandFlags.genStructureFlag:
			if err := gzcli.MustInit().GenerateStructure(); err != nil {
				log.Fatal("generate structure error: ", err)
			}

		default:
			cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(gzcliCmd)
	flags := gzcliCmd.Flags()

	flags.BoolVar(&commandFlags.initFlag, "init", false, "Initialize new CTF structure")
	flags.BoolVar(&commandFlags.genStructureFlag, "gen-structure", false, "generate structure for each challenge folder based on .structure")
	flags.BoolVar(&commandFlags.syncFlag, "sync", false, "Synchronize CTF data")
	flags.BoolVar(&commandFlags.ctftimeFlag, "ctftime-scoreboard", false, "Generate CTFTime scoreboard feed")
	flags.StringVar(&commandFlags.scriptFlag, "run-script", "", "Execute custom script")
	flags.StringVar(&commandFlags.createTeamsFlag, "create-teams", "", "Batch create teams")
	flags.StringVar(&commandFlags.createTeamsEmail, "create-teams-and-send-email", "", "Create teams and send emails")
	flags.BoolVar(&commandFlags.deleteUsersFlag, "delete-all-user", false, "Remove all users")
	flags.BoolVar(&commandFlags.updateGameFlag, "update-game", false, "Update the game")
}

func generateCTFTimeFeed(gz *gzcli.GZ) {
	feed := gz.MustScoreboard2CTFTimeFeed()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(feed); err != nil {
		log.Fatal(fmt.Errorf("JSON encoding failed: %w", err))
	}
}

func handleTeamCreation(url string, sendEmail bool) {
	if err := gzcli.MustInit().CreateTeams(url, sendEmail); err != nil {
		log.Fatal(err)
	}
}
