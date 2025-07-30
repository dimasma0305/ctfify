package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	// Init-related flags
	initUrl            string
	initPublicEntry    string
	initDiscordWebhook string
	// Watch-related flags
	watchRunFlag      bool
	watchStatusFlag   bool
	watchDebounce     time.Duration
	watchPollInterval time.Duration
	watchIgnore       []string
	watchPatterns     []string
	watchStatusJSON   bool
	// Git pull flags
	watchGitPull         bool
	watchGitPullInterval time.Duration
}

var commandFlags tcommandFlags

// gzcliCmd represents the optimized gzcli command
var gzcliCmd = &cobra.Command{
	Use:   "gzcli",
	Short: "High-performance CLI for gz::ctf",
	Long: `Optimized command line interface for gz::ctf operations with file watching capabilities.

For CTF initialization, you can provide values via flags or be prompted for input:
  --init-url              URL for the new CTF instance  
  --init-public-entry     Public entry point for the new CTF instance
  --init-discord-webhook  Discord webhook URL for notifications`,
	Run: func(cmd *cobra.Command, args []string) {
		switch {
		case commandFlags.initFlag:
			initInfo := map[string]string{
				"url":            commandFlags.initUrl,
				"publicEntry":    commandFlags.initPublicEntry,
				"discordWebhook": commandFlags.initDiscordWebhook,
			}
			if errors := other.CTFTemplate(".", initInfo); errors != nil {
				for _, err := range errors {
					if err != nil {
						log.Error("%s", err)
					}
				}
			}
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

		case commandFlags.watchRunFlag:
			handleWatchRun()

		case commandFlags.watchStatusFlag:
			handleWatchStatus()

		default:
			cmd.Help()
		}
	},
}

func init() {
	rootCmd.AddCommand(gzcliCmd)
	flags := gzcliCmd.Flags()

	// Original flags
	flags.BoolVar(&commandFlags.initFlag, "init", false, "Initialize new CTF structure (use with --init-url, --init-public-entry, --init-discord-webhook flags or be prompted for input)")
	flags.BoolVar(&commandFlags.genStructureFlag, "gen-structure", false, "generate structure for each challenge folder based on .structure")
	flags.BoolVar(&commandFlags.syncFlag, "sync", false, "Synchronize CTF data")
	flags.BoolVar(&commandFlags.ctftimeFlag, "ctftime-scoreboard", false, "Generate CTFTime scoreboard feed")
	flags.StringVar(&commandFlags.scriptFlag, "run-script", "", "Execute custom script")
	flags.StringVar(&commandFlags.createTeamsFlag, "create-teams", "", "Batch create teams")
	flags.StringVar(&commandFlags.createTeamsEmail, "create-teams-and-send-email", "", "Create teams and send emails")
	flags.BoolVar(&commandFlags.deleteUsersFlag, "delete-all-user", false, "Remove all users")
	flags.BoolVar(&commandFlags.updateGameFlag, "update-game", false, "Update the game")

	// Init-related flags
	flags.StringVar(&commandFlags.initUrl, "init-url", "", "URL for the new CTF instance")
	flags.StringVar(&commandFlags.initPublicEntry, "init-public-entry", "", "Public entry point for the new CTF instance")
	flags.StringVar(&commandFlags.initDiscordWebhook, "init-discord-webhook", "", "Discord webhook URL for notifications")

	// Watch-related flags
	flags.BoolVar(&commandFlags.watchRunFlag, "watch", false, "Run file watcher for automatic challenge redeployment")
	flags.BoolVar(&commandFlags.watchStatusFlag, "watch-status", false, "Show file watcher status")
	flags.DurationVar(&commandFlags.watchDebounce, "watch-debounce", 2*time.Second, "Debounce time for file changes")
	flags.DurationVar(&commandFlags.watchPollInterval, "watch-poll-interval", 5*time.Second, "Polling interval for file changes")
	flags.StringSliceVar(&commandFlags.watchIgnore, "watch-ignore", []string{}, "Additional patterns to ignore")
	flags.StringSliceVar(&commandFlags.watchPatterns, "watch-patterns", []string{}, "File patterns to watch (overrides default)")
	flags.BoolVar(&commandFlags.watchStatusJSON, "watch-status-json", false, "Output watch status in JSON format")

	// Git pull flags
	flags.BoolVar(&commandFlags.watchGitPull, "watch-git-pull", false, "Enable periodic git pull to sync repository")
	flags.DurationVar(&commandFlags.watchGitPullInterval, "watch-git-pull-interval", 5*time.Second, "Interval for git pull checks")
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

func handleWatchRun() {
	gz := gzcli.MustInit()

	// Configure watcher
	config := gzcli.WatcherConfig{
		PollInterval:              commandFlags.watchPollInterval,
		DebounceTime:              commandFlags.watchDebounce,
		IgnorePatterns:            gzcli.DefaultWatcherConfig.IgnorePatterns,
		WatchPatterns:             gzcli.DefaultWatcherConfig.WatchPatterns,
		EnableGitPull:             commandFlags.watchGitPull,
		GitPullInterval:           commandFlags.watchGitPullInterval,
		NewChallengeCheckInterval: gzcli.DefaultWatcherConfig.NewChallengeCheckInterval,
	}

	// Add custom ignore patterns if specified
	if len(commandFlags.watchIgnore) > 0 {
		config.IgnorePatterns = append(config.IgnorePatterns, commandFlags.watchIgnore...)
	}

	// Override watch patterns if specified
	if len(commandFlags.watchPatterns) > 0 {
		config.WatchPatterns = commandFlags.watchPatterns
	}

	log.Info("Starting file watcher...")
	if config.EnableGitPull {
		log.Info("Git pull enabled: checking every %v", config.GitPullInterval)
	}

	// Start the watcher
	if err := gz.StartWatcher(config); err != nil {
		log.Fatal("Failed to start watcher: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Info("File watcher running. Press Ctrl+C to stop.")
	<-sigChan

	log.Info("Shutting down file watcher...")
	if err := gz.StopWatcher(); err != nil {
		log.Error("Error stopping watcher: %v", err)
	}
	log.Info("File watcher stopped.")
}

func handleWatchStatus() {
	gz := gzcli.MustInit()

	status := gz.GetWatcherStatus()
	isRunning := status["running"].(bool)

	if isRunning {
		log.Info("File watcher service: RUNNING")
	} else {
		log.Info("File watcher service: STOPPED")
	}

	watchedChallenges := status["watched_challenges"].([]string)
	if len(watchedChallenges) > 0 {
		log.Info("Monitored challenge directories:")
		for _, dir := range watchedChallenges {
			log.InfoH2("- %s", dir)
		}
	} else {
		log.Info("No challenges are currently being monitored")
	}

	// Output JSON format if requested
	if commandFlags.watchStatusJSON {
		jsonData, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			log.Error("Failed to marshal status to JSON: %v", err)
			return
		}
		fmt.Println(string(jsonData))
	}
}
