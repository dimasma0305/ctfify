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
	// Watcher client flags
	watcherClientAction     string
	watcherClientChallenge  string
	watcherClientScript     string
	watcherClientLimit      int
	watcherClientSocketPath string
	watcherClientInterval   time.Duration
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
  --init-discord-webhook  Discord webhook URL for notifications

Watcher Client Commands:
  --watcher-client status                     Show watcher daemon status
  --watcher-client list                       List watched challenges
  --watcher-client logs                       View database logs
  --watcher-client live-logs                  Stream live database logs (use --watcher-interval to set refresh rate)
  --watcher-client metrics                    View script execution metrics
  --watcher-client executions                 View script execution history
  --watcher-client stop-script                Stop interval script (requires --watcher-challenge and --watcher-script)
  --watcher-client restart                    Restart challenge (requires --watcher-challenge)`,
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

		case commandFlags.watcherClientAction != "":
			handleWatcherClient()

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
	// Watcher client flags
	flags.StringVar(&commandFlags.watcherClientAction, "watcher-client", "", "Interact with running watcher daemon (status|list|logs|live-logs|metrics|stop-script|restart|executions)")
	flags.StringVar(&commandFlags.watcherClientChallenge, "watcher-challenge", "", "Challenge name for watcher client operations")
	flags.StringVar(&commandFlags.watcherClientScript, "watcher-script", "", "Script name for watcher client operations")
	flags.IntVar(&commandFlags.watcherClientLimit, "watcher-limit", 50, "Limit for logs/executions queries")
	flags.StringVar(&commandFlags.watcherClientSocketPath, "watcher-socket", "", "Custom watcher socket path")
	flags.DurationVar(&commandFlags.watcherClientInterval, "watcher-interval", 2*time.Second, "Refresh interval for live logs")
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
		DatabaseEnabled:           true,
		SocketEnabled:             true,
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

func handleWatcherClient() {
	client := gzcli.NewWatcherClient(commandFlags.watcherClientSocketPath)

	switch commandFlags.watcherClientAction {
	case "status":
		if err := client.PrintStatus(); err != nil {
			log.Fatal("Failed to get watcher status: ", err)
		}

	case "list":
		if err := client.PrintChallenges(); err != nil {
			log.Fatal("Failed to list challenges: ", err)
		}

	case "logs":
		if err := client.PrintLogs(commandFlags.watcherClientLimit); err != nil {
			log.Fatal("Failed to get logs: ", err)
		}

	case "live-logs":
		if err := client.StreamLiveLogs(commandFlags.watcherClientLimit, commandFlags.watcherClientInterval); err != nil {
			log.Fatal("Failed to stream live logs: ", err)
		}

	case "metrics":
		if err := client.PrintMetrics(); err != nil {
			log.Fatal("Failed to get metrics: ", err)
		}

	case "stop-script":
		if commandFlags.watcherClientChallenge == "" || commandFlags.watcherClientScript == "" {
			log.Fatal("Both --watcher-challenge and --watcher-script are required for stop-script action")
		}

		response, err := client.StopScript(commandFlags.watcherClientChallenge, commandFlags.watcherClientScript)
		if err != nil {
			log.Fatal("Failed to stop script: ", err)
		}

		if response.Success {
			log.Info("✅ %s", response.Message)
		} else {
			log.Error("❌ %s", response.Error)
		}

	case "restart":
		if commandFlags.watcherClientChallenge == "" {
			log.Fatal("--watcher-challenge is required for restart action")
		}

		response, err := client.RestartChallenge(commandFlags.watcherClientChallenge)
		if err != nil {
			log.Fatal("Failed to restart challenge: ", err)
		}

		if response.Success {
			log.Info("✅ %s", response.Message)
		} else {
			log.Error("❌ %s", response.Error)
		}

	case "executions":
		response, err := client.GetScriptExecutions(commandFlags.watcherClientChallenge, commandFlags.watcherClientLimit)
		if err != nil {
			log.Fatal("Failed to get script executions: ", err)
		}

		if !response.Success {
			log.Fatal("Get script executions failed: %s", response.Error)
		}

		fmt.Printf("📊 Script Executions (last %d entries)\n", commandFlags.watcherClientLimit)
		fmt.Println("==========================================")

		if data, ok := response.Data["executions"].([]interface{}); ok {
			if len(data) == 0 {
				fmt.Println("No script executions found.")
				return
			}

			for _, execInterface := range data {
				if execMap, ok := execInterface.(map[string]interface{}); ok {
					timestamp := ""
					if t, ok := execMap["timestamp"].(string); ok {
						if parsed, err := time.Parse("2006-01-02T15:04:05Z", t); err == nil {
							timestamp = parsed.Format("2006-01-02 15:04:05")
						} else {
							timestamp = t
						}
					}

					challenge := ""
					if c, ok := execMap["challenge"].(string); ok {
						challenge = c
					}

					scriptName := ""
					if s, ok := execMap["script_name"].(string); ok {
						scriptName = s
					}

					// Handle duration - check for both duration field and potential nil/zero values
					duration := ""
					var durationNs float64 = 0
					if d, ok := execMap["duration"].(float64); ok && d > 0 {
						durationNs = d
						if d >= 1000000000 { // >= 1 second
							duration = fmt.Sprintf("%.1fs", d/1000000000)
						} else if d >= 1000000 { // >= 1 millisecond
							duration = fmt.Sprintf("%.0fms", d/1000000)
						} else if d > 0 {
							duration = fmt.Sprintf("%.0fμs", d/1000)
						}
					}

					// Handle exit code - it might be nil for running scripts
					exitCode := ""
					hasExitCode := false
					if ec, ok := execMap["exit_code"]; ok && ec != nil {
						if exitCodeFloat, ok := ec.(float64); ok {
							exitCode = fmt.Sprintf(" (exit %d)", int(exitCodeFloat))
							hasExitCode = true
						}
					}

					// Determine success - use exit code if available, otherwise check success field
					success := "❌"
					if hasExitCode {
						// If we have an exit code, success is determined by exit code being 0
						if exitCode == " (exit 0)" {
							success = "✅"
						}
					} else if s, ok := execMap["success"].(bool); ok && s {
						// Fall back to success field if no exit code
						success = "✅"
					} else if durationNs == 0 {
						// If duration is 0, it might be a script that's still running or failed to start
						success = "⏳"
					}

					// Format output
					fmt.Printf("[%s] %s %s/%s", timestamp, success, challenge, scriptName)
					if duration != "" {
						fmt.Printf(" %s", duration)
					}
					if exitCode != "" {
						fmt.Printf("%s", exitCode)
					} else if success == "⏳" {
						fmt.Printf(" (running)")
					}
					fmt.Println()
				}
			}
		}

	default:
		log.Fatal("Invalid watcher client action. Valid actions: status, list, logs, live-logs, metrics, stop-script, restart, executions")
	}
}
