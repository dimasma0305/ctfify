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
	initFlag              bool
	syncFlag              bool
	ctftimeFlag           bool
	scriptFlag            string
	createTeamsFlag       string
	createTeamsEmail      string
	registerTeamsFlag     string
	registerTeamsEmail    string
	registerTeamsGame     string
	registerTeamsDivision string
	registerTeamsInvite   string
	deleteUsersFlag       bool
	updateGameFlag        bool
	genStructureFlag      bool
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
	watchForeground   bool
	watchStopFlag     bool
	watchLogsFlag     bool
	watchPidFile      string
	watchLogFile      string
	// Git-related flags
	watchGitPull         bool
	watchGitPullInterval time.Duration
	watchGitRepository   string
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

		case commandFlags.watchStopFlag:
			handleWatchStop()

		case commandFlags.watchLogsFlag:
			handleWatchLogs()

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
	flags.StringVar(&commandFlags.createTeamsFlag, "create-teams", "", "Batch create teams [RealName, Email, TeamName]")
	flags.StringVar(&commandFlags.createTeamsEmail, "create-teams-and-send-email", "", "Create teams and send emails [RealName, Email, TeamName]")
	flags.StringVar(&commandFlags.registerTeamsFlag, "register-teams", "", "Create teams from CSV and register them to CTF game (use with --game, --division, --invite)")
	flags.StringVar(&commandFlags.registerTeamsEmail, "register-teams-and-send-email", "", "Create teams from CSV, send emails, and register to CTF game (use with --game, --division, --invite)")
	flags.StringVar(&commandFlags.registerTeamsGame, "game", "", "Game title to register teams to (required for --register-teams)")
	flags.StringVar(&commandFlags.registerTeamsDivision, "division", "", "Division for team registration (optional)")
	flags.StringVar(&commandFlags.registerTeamsInvite, "invite", "", "Invitation code for team registration (optional)")
	flags.BoolVar(&commandFlags.deleteUsersFlag, "delete-all-user", false, "Remove all users")
	flags.BoolVar(&commandFlags.updateGameFlag, "update-game", false, "Update the game")

	// Init-related flags
	flags.StringVar(&commandFlags.initUrl, "init-url", "", "URL for the new CTF instance")
	flags.StringVar(&commandFlags.initPublicEntry, "init-public-entry", "", "Public entry point for the new CTF instance")
	flags.StringVar(&commandFlags.initDiscordWebhook, "init-discord-webhook", "", "Discord webhook URL for notifications")

	// Watch-related flags
	flags.BoolVar(&commandFlags.watchRunFlag, "watch", false, "Run file watcher for automatic challenge redeployment (daemon mode by default)")
	flags.BoolVar(&commandFlags.watchStatusFlag, "watch-status", false, "Show file watcher status")
	flags.BoolVar(&commandFlags.watchStopFlag, "watch-stop", false, "Stop the daemon watcher")
	flags.BoolVar(&commandFlags.watchLogsFlag, "watch-logs", false, "Follow and display daemon log file in real-time")
	flags.BoolVar(&commandFlags.watchForeground, "watch-foreground", false, "Run watcher in foreground instead of daemon mode")
	flags.StringVar(&commandFlags.watchPidFile, "watch-pid-file", "", "Custom PID file location (default: /tmp/gzctf-watcher.pid)")
	flags.StringVar(&commandFlags.watchLogFile, "watch-log-file", "", "Custom log file location (default: /tmp/gzctf-watcher.log)")
	flags.DurationVar(&commandFlags.watchDebounce, "watch-debounce", 2*time.Second, "Debounce time for file changes")
	flags.DurationVar(&commandFlags.watchPollInterval, "watch-poll-interval", 5*time.Second, "Polling interval for file changes")
	flags.StringSliceVar(&commandFlags.watchIgnore, "watch-ignore", []string{}, "Additional patterns to ignore")
	flags.StringSliceVar(&commandFlags.watchPatterns, "watch-patterns", []string{}, "File patterns to watch (overrides default)")
	flags.BoolVar(&commandFlags.watchStatusJSON, "watch-status-json", false, "Output watch status in JSON format")
	// Git-related flags
	flags.BoolVar(&commandFlags.watchGitPull, "watch-git-pull", true, "Enable automatic git pull (default: true)")
	flags.DurationVar(&commandFlags.watchGitPullInterval, "watch-git-pull-interval", 1*time.Minute, "Git pull interval")
	flags.StringVar(&commandFlags.watchGitRepository, "watch-git-repository", ".", "Git repository path to pull from")

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
		NewChallengeCheckInterval: gzcli.DefaultWatcherConfig.NewChallengeCheckInterval,
		DaemonMode:                !commandFlags.watchForeground, // Daemon by default, unless foreground is requested
		PidFile:                   gzcli.DefaultWatcherConfig.PidFile,
		LogFile:                   gzcli.DefaultWatcherConfig.LogFile,
		GitPullEnabled:            commandFlags.watchGitPull,
		GitPullInterval:           commandFlags.watchGitPullInterval,
		GitRepository:             commandFlags.watchGitRepository,
	}

	// Override daemon settings if custom files specified
	if commandFlags.watchPidFile != "" {
		config.PidFile = commandFlags.watchPidFile
	}
	if commandFlags.watchLogFile != "" {
		config.LogFile = commandFlags.watchLogFile
	}

	// Add custom ignore patterns if specified
	if len(commandFlags.watchIgnore) > 0 {
		config.IgnorePatterns = append(config.IgnorePatterns, commandFlags.watchIgnore...)
	}

	// Override watch patterns if specified
	if len(commandFlags.watchPatterns) > 0 {
		config.WatchPatterns = commandFlags.watchPatterns
	}

	if config.DaemonMode {
		log.Info("Starting file watcher as daemon...")
	} else {
		log.Info("Starting file watcher in foreground...")
	}

	// Start the watcher
	if err := gz.StartWatcher(config); err != nil {
		log.Fatal("Failed to start watcher: ", err)
	}

	// If running in foreground mode, set up signal handling
	if !config.DaemonMode {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		log.Info("File watcher running in foreground. Press Ctrl+C to stop.")
		<-sigChan

		log.Info("Shutting down file watcher...")
		if err := gz.StopWatcher(); err != nil {
			log.Error("Error stopping watcher: %v", err)
		}
		log.Info("File watcher stopped.")
	}
}

func handleWatchStatus() {
	gz := gzcli.MustInit()

	// Create a watcher instance to access status methods
	watcher, err := gzcli.NewWatcher(gz)
	if err != nil {
		log.Fatal("Failed to create watcher: ", err)
	}

	// Get file paths
	pidFile := gzcli.DefaultWatcherConfig.PidFile
	if commandFlags.watchPidFile != "" {
		pidFile = commandFlags.watchPidFile
	}

	logFile := gzcli.DefaultWatcherConfig.LogFile
	if commandFlags.watchLogFile != "" {
		logFile = commandFlags.watchLogFile
	}

	// Use watcher's status method
	if err := watcher.ShowStatus(pidFile, logFile, commandFlags.watchStatusJSON); err != nil {
		log.Error("Failed to show status: %v", err)
	}
}

func handleWatchStop() {
	gz := gzcli.MustInit()

	// Create a watcher instance to access stop methods
	watcher, err := gzcli.NewWatcher(gz)
	if err != nil {
		log.Fatal("Failed to create watcher: ", err)
	}

	pidFile := gzcli.DefaultWatcherConfig.PidFile
	if commandFlags.watchPidFile != "" {
		pidFile = commandFlags.watchPidFile
	}

	log.Info("🛑 Stopping GZCTF Watcher daemon...")
	if err := watcher.StopDaemon(pidFile); err != nil {
		log.Fatal("Failed to stop daemon: ", err)
	}
}

func handleWatchLogs() {
	gz := gzcli.MustInit()

	// Create a watcher instance to access log methods
	watcher, err := gzcli.NewWatcher(gz)
	if err != nil {
		log.Fatal("Failed to create watcher: ", err)
	}

	// Get log file path
	logFile := gzcli.DefaultWatcherConfig.LogFile
	if commandFlags.watchLogFile != "" {
		logFile = commandFlags.watchLogFile
	}

	log.Info("📋 Following GZCTF Watcher logs: %s", logFile)
	log.Info("Press Ctrl+C to stop following logs")
	log.Info("==========================================")

	// Follow the log file
	if err := watcher.FollowLogs(logFile); err != nil {
		log.Fatal("Failed to follow logs: ", err)
	}
}
