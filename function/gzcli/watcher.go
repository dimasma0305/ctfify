package gzcli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/fsnotify/fsnotify"
	tail "github.com/hpcloud/tail"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sevlyar/go-daemon"
)

// ScriptMetrics tracks execution statistics for scripts
type ScriptMetrics struct {
	LastExecution  time.Time
	ExecutionCount int64
	LastError      error
	LastDuration   time.Duration
	TotalDuration  time.Duration
	Interval       time.Duration `json:"interval,omitempty"` // For interval scripts
	IsInterval     bool          `json:"is_interval"`        // Whether this is an interval script
}

// WatcherCommand represents commands that can be sent to the watcher via socket
type WatcherCommand struct {
	Action string                 `json:"action"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// WatcherResponse represents responses from the watcher
type WatcherResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// Database models for persistent storage
type WatcherLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Challenge string    `json:"challenge,omitempty"`
	Script    string    `json:"script,omitempty"`
	Message   string    `json:"message"`
	Error     string    `json:"error,omitempty"`
	Duration  int64     `json:"duration,omitempty"` // milliseconds
}

type ChallengeState struct {
	ID            int64     `json:"id"`
	ChallengeName string    `json:"challenge_name"`
	Status        string    `json:"status"` // watching, updating, deploying, error
	LastUpdate    time.Time `json:"last_update"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	ScriptStates  string    `json:"script_states"` // JSON of active interval scripts
}

type ScriptExecution struct {
	ID            int64     `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	ChallengeName string    `json:"challenge_name"`
	ScriptName    string    `json:"script_name"`
	ScriptType    string    `json:"script_type"` // one-time, interval
	Command       string    `json:"command"`
	Status        string    `json:"status"`             // started, completed, failed, cancelled
	Duration      int64     `json:"duration,omitempty"` // nanoseconds
	Output        string    `json:"output,omitempty"`
	ErrorOutput   string    `json:"error_output,omitempty"`
	ExitCode      int       `json:"exit_code,omitempty"`
	Success       bool      `json:"success"` // computed field based on status and exit code
}

type Watcher struct {
	gz                   *GZ
	watcher              *fsnotify.Watcher
	config               WatcherConfig
	ctx                  context.Context
	cancel               context.CancelFunc
	wg                   sync.WaitGroup
	debounceTimers       map[string]*time.Timer
	debounceTimersMu     sync.RWMutex
	challengeMutexes     map[string]*sync.Mutex
	challengeMutexesMu   sync.RWMutex
	pendingUpdates       map[string]string // challengeName -> latest file path
	pendingUpdatesMu     sync.RWMutex
	updatingChallenges   map[string]bool // challengeName -> is updating
	updatingMu           sync.RWMutex
	watchedChallenges    map[string]bool // challengeName -> is being watched
	watchedMu            sync.RWMutex
	daemonContext        *daemon.Context          // Daemon context for process management
	watchedChallengeDirs map[string]string        // challengeName -> cwd
	challengeConfigs     map[string]ChallengeYaml // challengeName -> full config (for scripts)
	challengeConfigsMu   sync.RWMutex
	intervalScripts      map[string]map[string]context.CancelFunc // challengeName -> scriptName -> cancelFunc
	intervalScriptsMu    sync.RWMutex
	scriptMetrics        map[string]map[string]*ScriptMetrics // challengeName -> scriptName -> metrics
	scriptMetricsMu      sync.RWMutex
	// Database and socket related fields
	db           *sql.DB
	dbMu         sync.RWMutex
	socketPath   string
	socketServer net.Listener
	socketMu     sync.RWMutex
}

type WatcherConfig struct {
	PollInterval              time.Duration
	DebounceTime              time.Duration
	IgnorePatterns            []string
	WatchPatterns             []string
	NewChallengeCheckInterval time.Duration // New field for checking new challenges
	DaemonMode                bool          // Run watcher as daemon
	PidFile                   string        // PID file location
	LogFile                   string        // Log file location
	GitPullEnabled            bool          // Enable automatic git pull
	GitPullInterval           time.Duration // Interval for git pull (default: 1 minute)
	GitRepository             string        // Git repository path (default: current directory)
	// Database configuration
	DatabaseEnabled bool   // Enable database logging
	DatabasePath    string // SQLite database file path
	// Socket configuration
	SocketEnabled bool   // Enable socket server
	SocketPath    string // Unix socket path for communication
}

var DefaultWatcherConfig = WatcherConfig{
	PollInterval:              5 * time.Second,
	DebounceTime:              2 * time.Second,
	IgnorePatterns:            []string{},       // No ignore patterns
	WatchPatterns:             []string{},       // Empty means watch all files
	NewChallengeCheckInterval: 10 * time.Second, // Check for new challenges every 10 seconds
	DaemonMode:                true,             // Default to daemon mode
	PidFile:                   "/tmp/gzctf-watcher.pid",
	LogFile:                   "/tmp/gzctf-watcher.log",
	GitPullEnabled:            true,            // Enable git pull by default
	GitPullInterval:           1 * time.Minute, // Pull every minute
	GitRepository:             ".",             // Current directory
	// Database defaults
	DatabaseEnabled: true, // Enable database logging by default
	DatabasePath:    "/tmp/gzctf-watcher.db",
	// Socket defaults
	SocketEnabled: true, // Enable socket server by default
	SocketPath:    "/tmp/gzctf-watcher.sock",
}

// getChallengeUpdateMutex gets or creates a mutex for a specific challenge
func (w *Watcher) getChallengeUpdateMutex(challengeName string) *sync.Mutex {
	w.challengeMutexesMu.RLock()
	if mutex, exists := w.challengeMutexes[challengeName]; exists {
		w.challengeMutexesMu.RUnlock()
		return mutex
	}
	w.challengeMutexesMu.RUnlock()

	// Need to create new mutex
	w.challengeMutexesMu.Lock()
	defer w.challengeMutexesMu.Unlock()

	// Double-check in case another goroutine created it
	if mutex, exists := w.challengeMutexes[challengeName]; exists {
		return mutex
	}

	// Create new mutex
	mutex := &sync.Mutex{}
	w.challengeMutexes[challengeName] = mutex
	return mutex
}

// NewWatcher creates a new file watcher instance
func NewWatcher(gz *GZ) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		gz:                   gz,
		watcher:              watcher,
		ctx:                  ctx,
		cancel:               cancel,
		debounceTimers:       make(map[string]*time.Timer),
		challengeMutexes:     make(map[string]*sync.Mutex),
		pendingUpdates:       make(map[string]string),
		updatingChallenges:   make(map[string]bool),
		watchedChallenges:    make(map[string]bool),
		watchedChallengeDirs: make(map[string]string),
		challengeConfigs:     make(map[string]ChallengeYaml),
		intervalScripts:      make(map[string]map[string]context.CancelFunc),
		scriptMetrics:        make(map[string]map[string]*ScriptMetrics),
		// Database and socket will be initialized in Start() if enabled
		db:           nil,
		socketServer: nil,
		socketPath:   "",
	}, nil
}

// Start starts the file watcher with the given configuration
func (w *Watcher) Start(config WatcherConfig) error {
	w.config = config

	// Validate and set defaults for zero durations
	if w.config.NewChallengeCheckInterval <= 0 {
		w.config.NewChallengeCheckInterval = DefaultWatcherConfig.NewChallengeCheckInterval
		log.Info("NewChallengeCheckInterval was zero or negative, using default: %v", w.config.NewChallengeCheckInterval)
	}

	// Set default paths if not provided
	if w.config.PidFile == "" {
		w.config.PidFile = DefaultWatcherConfig.PidFile
	}
	if w.config.LogFile == "" {
		w.config.LogFile = DefaultWatcherConfig.LogFile
	}
	if w.config.DatabasePath == "" {
		w.config.DatabasePath = DefaultWatcherConfig.DatabasePath
	}
	if w.config.SocketPath == "" {
		w.config.SocketPath = DefaultWatcherConfig.SocketPath
	}

	if w.config.DaemonMode {
		log.Info("Starting file watcher in DAEMON mode...")
		return w.startAsDaemon()
	}

	log.Info("Starting file watcher in foreground mode...")
	return w.startWatcher()
}

// startAsDaemon starts the watcher as a daemon process
func (w *Watcher) startAsDaemon() error {
	// Create daemon context
	w.daemonContext = &daemon.Context{
		PidFileName: w.config.PidFile,
		PidFilePerm: 0644,
		LogFileName: w.config.LogFile,
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
		Args:        nil, // Don't override command args - let daemon handle it
	}

	// Check if we're already in the daemon process
	if daemon.WasReborn() {
		// This is the child daemon process
		pid := os.Getpid()
		log.Info("🚀 GZCTF Watcher daemon started (PID: %d)", pid)
		log.Info("📄 PID file: %s", w.config.PidFile)
		log.Info("📝 Log file: %s", w.config.LogFile)

		// Manually write PID file since go-daemon might not be doing it correctly
		if err := w.writePIDFile(w.config.PidFile, pid); err != nil {
			log.Error("Failed to write PID file: %v", err)
			return fmt.Errorf("failed to write PID file: %w", err)
		}

		// Start the actual watcher and keep it running
		if err := w.startWatcher(); err != nil {
			return err
		}
		// Keep daemon running until context is cancelled
		<-w.ctx.Done()
		return nil
	}

	// This is the parent process - fork the daemon
	child, err := w.daemonContext.Reborn()
	if err != nil {
		return fmt.Errorf("failed to fork daemon: %w", err)
	}

	if child != nil {
		// Parent process - daemon started successfully
		log.Info("✅ GZCTF Watcher daemon started successfully")
		log.Info("📄 PID: %d (saved to %s)", child.Pid, w.config.PidFile)
		log.Info("📝 Logs: %s", w.config.LogFile)
		log.Info("🔧 Use 'gzcli --watch-status' to check status")
		log.Info("🛑 Use daemon control methods to stop")
		return nil
	}

	// This should not be reached
	return fmt.Errorf("unexpected daemon state")
}

// startWatcher starts the actual watcher functionality
func (w *Watcher) startWatcher() error {
	// Initialize database if enabled
	if err := w.initDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize socket server if enabled
	if err := w.initSocketServer(); err != nil {
		return fmt.Errorf("failed to initialize socket server: %w", err)
	}

	// Get challenges and add them to watch
	challenges, err := w.getChallenges()
	if err != nil {
		return fmt.Errorf("failed to get challenges: %w", err)
	}

	for _, challenge := range challenges {
		if err := w.addChallengeToWatch(challenge); err != nil {
			log.Error("Failed to watch challenge %s: %v", challenge.Name, err)
			w.logToDatabase("ERROR", "watcher", challenge.Name, "", "Failed to add challenge to watcher", err.Error(), 0)
			continue
		}
		// Log initial challenge state
		w.updateChallengeState(challenge.Name, "watching", "")
		w.logToDatabase("INFO", "watcher", challenge.Name, "", "Challenge added to watcher successfully", "", 0)
	}

	// Start the main watch loop
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.watchLoop(w.config)
	}()

	// Start new challenge checking routine
	log.Info("New challenge detection enabled, checking every %v", w.config.NewChallengeCheckInterval)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.newChallengeCheckLoop(w.config)
	}()

	// Start git pull routine if enabled
	if w.config.GitPullEnabled {
		log.Info("Git pull enabled, pulling every %v from %s", w.config.GitPullInterval, w.config.GitRepository)
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.gitPullLoop(w.config)
		}()
	}

	// Start socket server if enabled
	if w.config.SocketEnabled {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.socketServerLoop()
		}()
	}

	log.Info("📁 Watching %d challenges", len(challenges))
	log.Info("File watcher started successfully")

	// Log watcher startup
	w.logToDatabase("INFO", "watcher", "", "", fmt.Sprintf("File watcher started, watching %d challenges", len(challenges)), "", 0)

	return nil
}

// stopAllIntervalScriptsWithTimeout stops all interval scripts with a timeout
func (w *Watcher) stopAllIntervalScriptsWithTimeout(timeout time.Duration) {
	log.Info("Stopping all interval scripts with timeout %v...", timeout)

	w.intervalScriptsMu.Lock()
	defer w.intervalScriptsMu.Unlock()

	if len(w.intervalScripts) == 0 {
		return
	}

	// Cancel all scripts
	for challengeName := range w.intervalScripts {
		log.InfoH3("Stopping all interval scripts for challenge '%s'", challengeName)
		for scriptName, cancel := range w.intervalScripts[challengeName] {
			log.InfoH3("  - Stopping interval script '%s'", scriptName)
			cancel()
		}
	}

	// Clear all tracking
	w.intervalScripts = make(map[string]map[string]context.CancelFunc)

	// Give scripts time to finish
	if timeout > 0 {
		log.InfoH3("Waiting up to %v for scripts to finish...", timeout)
		time.Sleep(timeout)
	}
}

// Stop stops the file watcher with improved graceful shutdown
func (w *Watcher) Stop() error {
	log.Info("Stopping file watcher...")

	// Log shutdown start
	w.logToDatabase("INFO", "watcher", "", "", "File watcher shutdown initiated", "", 0)

	// Stop all interval scripts with timeout
	w.stopAllIntervalScriptsWithTimeout(5 * time.Second)

	// Cancel context after stopping interval scripts
	w.cancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.InfoH3("All goroutines finished successfully")
	case <-time.After(10 * time.Second):
		log.Error("Timeout waiting for goroutines to finish")
	}

	// Close socket server
	if err := w.closeSocketServer(); err != nil {
		log.Error("Failed to close socket server: %v", err)
	}

	// Close file system watcher
	if w.watcher != nil {
		err := w.watcher.Close()
		if err != nil {
			log.Error("Failed to close file watcher: %v", err)
		}
	}

	// Log shutdown completion before closing database
	w.logToDatabase("INFO", "watcher", "", "", "File watcher shutdown completed", "", 0)

	// Close database connection last
	if err := w.closeDatabase(); err != nil {
		log.Error("Failed to close database: %v", err)
	}

	log.Info("File watcher stopped")
	return nil
}

// findChallengeByName finds a challenge by its name
func (w *Watcher) findChallengeByName(challengeName string) (*ChallengeYaml, error) {
	challenges, err := w.getChallenges()
	if err != nil {
		return nil, err
	}

	for _, challenge := range challenges {
		if challenge.Name == challengeName {
			return &challenge, nil
		}
	}

	return nil, nil // Challenge not found
}

// addChallengeToWatch adds a challenge directory to the watcher
func (w *Watcher) addChallengeToWatch(challenge ChallengeYaml) error {
	// Check if already watching this challenge
	w.watchedMu.RLock()
	if w.watchedChallenges[challenge.Name] {
		w.watchedMu.RUnlock()
		return nil // Already watching
	}
	w.watchedMu.RUnlock()

	// Add the challenge directory
	err := w.watcher.Add(challenge.Cwd)
	if err != nil {
		return fmt.Errorf("failed to add directory %s: %w", challenge.Cwd, err)
	}

	// Also watch subdirectories
	err = filepath.Walk(challenge.Cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() && !w.shouldIgnoreDir(path) {
			if err := w.watcher.Add(path); err != nil {
				log.Error("Failed to watch directory %s: %v", path, err)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory %s: %w", challenge.Cwd, err)
	}

	// Mark as watched and store config
	w.watchedMu.Lock()
	w.watchedChallenges[challenge.Name] = true
	w.watchedChallengeDirs[challenge.Name] = challenge.Cwd
	w.watchedMu.Unlock()

	w.challengeConfigsMu.Lock()
	w.challengeConfigs[challenge.Name] = challenge
	w.challengeConfigsMu.Unlock()

	log.InfoH2("Now watching: %s (%s)", challenge.Name, challenge.Cwd)
	return nil
}

// watchLoop is the main event loop for file watching
func (w *Watcher) watchLoop(config WatcherConfig) {

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if w.shouldProcessEvent(event, config) {
				log.InfoH2("File change detected: %s (%s)", event.Name, event.Op.String())

				// Handle removal events immediately without debouncing
				if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					log.InfoH2("Processing removal event immediately: %s", event.Name)
					w.handleEvent(event)
				} else {
					// Debounce other events (Write, Create)
					w.debounceTimersMu.Lock()
					if timer, exists := w.debounceTimers[event.Name]; exists {
						timer.Stop()
					}

					// Copy event to avoid closure capturing issues
					ev := event
					w.debounceTimers[event.Name] = time.AfterFunc(config.DebounceTime, func() {
						w.handleEvent(ev)
						w.debounceTimersMu.Lock()
						delete(w.debounceTimers, ev.Name)
						w.debounceTimersMu.Unlock()
					})
					w.debounceTimersMu.Unlock()
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error("Watcher error: %v", err)
		}
	}
}

// handleEvent routes fsnotify events to change or removal handlers
func (w *Watcher) handleEvent(event fsnotify.Event) {
	// On Remove or Rename, handle potential deletion
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.handleFileRemoval(event.Name)
		return
	}
	// For Create/Write, proceed with normal change handling if the file exists
	if _, err := os.Stat(event.Name); err == nil {
		w.handleFileChange(event.Name)
	}
}

// handleFileRemoval handles file or directory removals that may indicate a challenge deletion
func (w *Watcher) handleFileRemoval(path string) {
	// Check if the removed path is a directory that might be a challenge directory
	w.watchedMu.RLock()
	var isWatchedChallengeDir bool
	var challengeNameForDir string
	absPath, _ := filepath.Abs(path)

	for challengeName, challengeDir := range w.watchedChallengeDirs {
		absChallengeDir, _ := filepath.Abs(challengeDir)
		if absChallengeDir == absPath {
			isWatchedChallengeDir = true
			challengeNameForDir = challengeName
			break
		}
	}
	w.watchedMu.RUnlock()

	if isWatchedChallengeDir {
		log.InfoH2("Challenge directory removed: %s", challengeNameForDir)
		w.handleChallengeRemovalByDir(path)
		return
	}

	// If a challenge.yml or challenge.yaml is removed, infer which challenge it belonged to by path prefix
	base := filepath.Base(path)
	if base == "challenge.yml" || base == "challenge.yaml" {
		// The parent directory represents the challenge cwd
		dir := filepath.Dir(path)
		w.handleChallengeRemovalByDir(dir)
		return
	}
}

// handleChallengeRemovalByDir determines if a watched challenge lives under removedDir and undeploys+removes it
func (w *Watcher) handleChallengeRemovalByDir(removedDir string) {
	absRemoved, _ := filepath.Abs(removedDir)

	// Check if the directory itself was removed (directory deletion scenario)
	if _, err := os.Stat(absRemoved); os.IsNotExist(err) {
		// Directory no longer exists, proceed with removal
	} else {
		// Directory still exists, check if challenge files are missing
		chalYml := filepath.Join(absRemoved, "challenge.yml")
		chalYaml := filepath.Join(absRemoved, "challenge.yaml")
		if !(os.IsNotExist(fileStat(chalYml)) && os.IsNotExist(fileStat(chalYaml))) {
			return // Challenge files still exist, don't remove
		}
	}

	// Use the watcher's internal state to find the challenge that was removed
	// This is more reliable than getChallenges() since the YAML file is already deleted
	w.watchedMu.RLock()
	var removedChallengeName string
	for challengeName, challengeDir := range w.watchedChallengeDirs {
		absChallengeDir, _ := filepath.Abs(challengeDir)
		if absChallengeDir == absRemoved {
			removedChallengeName = challengeName
			break
		}
	}
	w.watchedMu.RUnlock()

	if removedChallengeName != "" {
		log.InfoH2("🗑️ Removing challenge: %s", removedChallengeName)

		// Get the stored challenge configuration for scripts
		w.challengeConfigsMu.RLock()
		removedChallenge, hasStoredConfig := w.challengeConfigs[removedChallengeName]
		w.challengeConfigsMu.RUnlock()

		if !hasStoredConfig {
			// Fallback to minimal challenge struct if no stored config
			removedChallenge = ChallengeYaml{
				Name: removedChallengeName,
				Cwd:  absRemoved,
			}
			log.Error("No stored configuration found for removed challenge %s, stop script may not run", removedChallengeName)
		}

		go w.undeployAndRemoveChallenge(removedChallenge)
		return
	}

	// If not found in watched challenges, try best-effort API deletion by path inference
	w.deleteApiChallengeByPath(removedDir)
}

// fileStat returns error if path does not exist; helper to simplify logic
func fileStat(path string) error {
	_, err := os.Stat(path)
	return err
}

// clearWatchedByPath clears local watched state using a removed directory path hint
func (w *Watcher) clearWatchedByPath(removedDir string) {
	w.watchedMu.Lock()
	defer w.watchedMu.Unlock()
	absRemoved, _ := filepath.Abs(removedDir)
	for name, dir := range w.watchedChallengeDirs {
		absDir, _ := filepath.Abs(dir)
		if strings.HasPrefix(absDir, absRemoved) || strings.HasPrefix(absRemoved, absDir) {
			delete(w.watchedChallenges, name)
			delete(w.watchedChallengeDirs, name)
			if w.watcher != nil {
				if err := w.watcher.Remove(dir); err != nil {
					log.DebugH3("Watcher remove for %s returned: %v", dir, err)
				}
			}
			log.InfoH3("Cleared watch state for removed challenge path: %s (%s)", name, dir)
		}
	}
}

// deleteApiChallengeByPath attempts to delete a challenge from API based on removed directory path
func (w *Watcher) deleteApiChallengeByPath(removedDir string) {
	absRemoved, _ := filepath.Abs(removedDir)
	candName := filepath.Base(absRemoved)
	candCategory := filepath.Base(filepath.Dir(absRemoved))

	config, err := GetConfig(w.gz.api)
	if err != nil {
		log.Error("Failed to load config to delete challenge by path %s: %v", removedDir, err)
		return
	}
	config.Event.CS = w.gz.api
	apiChallenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("Failed to fetch API challenges to delete by path %s: %v", removedDir, err)
		return
	}

	// Try to find best match by title (and category if matches)
	for i := range apiChallenges {
		titleMatch := strings.EqualFold(apiChallenges[i].Title, candName)
		categoryMatch := strings.EqualFold(apiChallenges[i].Category, candCategory)
		if titleMatch || (titleMatch && categoryMatch) {
			apiChallenges[i].CS = w.gz.api
			if err := apiChallenges[i].Delete(); err != nil {
				log.Error("Failed to delete challenge %s inferred from %s: %v", apiChallenges[i].Title, removedDir, err)
				continue
			}
			log.InfoH2("✅ Challenge removed from GZCTF by path inference: %s", apiChallenges[i].Title)
			// Best-effort: stop after first successful delete
			return
		}
	}

	log.Info("No matching API challenge found to delete for path: %s (candidate name: %s, category: %s)", removedDir, candName, candCategory)
}

// undeployAndRemoveChallenge stops running services and deletes the challenge from GZCTF
func (w *Watcher) undeployAndRemoveChallenge(challenge ChallengeYaml) {
	// Serialize per-challenge operations
	mutex := w.getChallengeUpdateMutex(challenge.Name)
	mutex.Lock()
	defer mutex.Unlock()

	log.InfoH2("🔻 Undeploying and removing challenge: %s", challenge.Name)

	// Stop all interval scripts for this challenge first
	w.stopAllIntervalScripts(challenge.Name)

	// Run stop script if available
	if scriptValue, exists := challenge.Scripts["stop"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running stop script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "stop"); err != nil {
			log.Error("Stop script failed for %s: %v", challenge.Name, err)
		}
	}

	// Delete from API
	config, err := GetConfig(w.gz.api)
	if err != nil {
		log.Error("Failed to load config to delete challenge %s: %v", challenge.Name, err)
		return
	}
	config.Event.CS = w.gz.api
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("Failed to list API challenges to delete %s: %v", challenge.Name, err)
		return
	}
	// Find API challenge
	var apiChallenge *gzapi.Challenge
	for i := range challenges {
		if challenges[i].Title == challenge.Name {
			apiChallenge = &challenges[i]
			break
		}
	}
	if apiChallenge == nil {
		log.Info("Challenge %s not present in API; nothing to delete", challenge.Name)
		return
	}
	apiChallenge.CS = w.gz.api
	if err := apiChallenge.Delete(); err != nil {
		log.Error("Failed to delete challenge %s in API: %v", challenge.Name, err)
		return
	}
	log.InfoH2("✅ Challenge removed from GZCTF: %s", challenge.Name)

	// Unwatch this challenge locally so it can be re-added if recreated later
	w.watchedMu.Lock()
	delete(w.watchedChallenges, challenge.Name)
	delete(w.watchedChallengeDirs, challenge.Name)
	w.watchedMu.Unlock()

	// Remove stored configuration
	w.challengeConfigsMu.Lock()
	delete(w.challengeConfigs, challenge.Name)
	w.challengeConfigsMu.Unlock()
	// Best-effort: remove watch on its cwd
	if w.watcher != nil {
		if err := w.watcher.Remove(challenge.Cwd); err != nil {
			// Directory may no longer exist; ignore errors
			log.DebugH3("Watcher remove for %s returned: %v", challenge.Cwd, err)
		}
	}
}

// shouldProcessEvent determines if we should process a file system event
func (w *Watcher) shouldProcessEvent(event fsnotify.Event, config WatcherConfig) bool {
	// Process Write, Create, Remove, and Rename events
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	filename := filepath.Base(event.Name)

	// Skip common editor temporary files that cause loops
	if strings.HasPrefix(filename, ".") && (strings.HasSuffix(filename, ".swp") ||
		strings.HasSuffix(filename, ".tmp") ||
		strings.HasSuffix(filename, "~") ||
		strings.Contains(filename, ".sw")) {
		return false
	}

	// Skip VSCode temporary files
	if strings.HasPrefix(filename, ".vscode") || strings.Contains(event.Name, ".vscode") {
		return false
	}

	// Check ignore patterns (if any)
	for _, pattern := range config.IgnorePatterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return false
		}
		if strings.Contains(event.Name, pattern) {
			return false
		}
	}

	// Check if it matches watch patterns (if specified)
	if len(config.WatchPatterns) > 0 {
		for _, pattern := range config.WatchPatterns {
			if matched, _ := filepath.Match(pattern, filename); matched {
				return true
			}
		}
		return false
	}

	return true
}

// shouldIgnoreDir determines if a directory should be ignored
func (w *Watcher) shouldIgnoreDir(path string) bool {
	// Only ignore hidden directories that start with dot (except current dir)
	dirName := filepath.Base(path)
	if strings.HasPrefix(dirName, ".") && dirName != "." && dirName != ".." {
		return true
	}
	return false
}

// UpdateType represents the type of update needed based on file changes
type UpdateType int

const (
	UpdateNone UpdateType = iota
	UpdateAttachment
	UpdateMetadata
	UpdateFullRedeploy
)

// determineUpdateType determines what type of update is needed based on the changed file
func (w *Watcher) determineUpdateType(filePath string, challenge ChallengeYaml) UpdateType {
	// Get relative path from challenge directory
	absChallengePath, err := filepath.Abs(challenge.Cwd)
	if err != nil {
		log.Error("Failed to get absolute challenge path: %v", err)
		return UpdateFullRedeploy // Default to full redeploy on error
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		log.Error("Failed to get absolute file path: %v", err)
		return UpdateFullRedeploy // Default to full redeploy on error
	}

	relPath, err := filepath.Rel(absChallengePath, absFilePath)
	if err != nil {
		log.Error("Failed to get relative path: %v", err)
		return UpdateFullRedeploy // Default to full redeploy on error
	}

	// Check if it's in solver directory - no update needed
	if strings.HasPrefix(relPath, "solver/") || strings.HasPrefix(relPath, "writeup/") {
		log.InfoH3("File is in solver/writeup directory, skipping update")
		return UpdateNone
	}

	// Check if it's challenge.yml or challenge.yaml - metadata update only
	base := filepath.Base(relPath)
	if base == "challenge.yml" || base == "challenge.yaml" {
		log.InfoH3("Challenge configuration file changed, updating metadata and attachment")
		return UpdateMetadata
	}

	// Check if it's in dist directory - attachment update only
	if strings.HasPrefix(relPath, "dist/") {
		log.InfoH3("File in dist directory changed, updating attachment only")
		return UpdateAttachment
	}

	// Check if it's in src directory - full redeploy needed
	if strings.HasPrefix(relPath, "src/") {
		log.InfoH3("Source file changed, full redeploy needed")
		return UpdateFullRedeploy
	}

	// Check for other important files that need full redeploy
	fileName := filepath.Base(relPath)
	if fileName == "Dockerfile" || fileName == "docker-compose.yml" || fileName == "Makefile" {
		log.InfoH3("Infrastructure file changed (%s), full redeploy needed", fileName)
		return UpdateFullRedeploy
	}

	// Only listen to src/, dist/ and challenge.yml/yaml. Ignore any other paths.
	log.InfoH3("Change outside allowed paths (src/, dist/, challenge.yml/.yaml); ignoring")
	return UpdateNone
}

// handleFileChange processes a file change event
func (w *Watcher) handleFileChange(filePath string) {
	log.InfoH2("Processing file change: %s", filePath)

	// Find which challenge this file belongs to
	challenge, err := w.findChallengeForFile(filePath)
	if err != nil {
		log.Error("Failed to find challenge for file %s: %v", filePath, err)
		return
	}

	if challenge == nil {
		log.InfoH3("File %s doesn't belong to any challenge", filePath)
		return
	}

	log.Info("File %s belongs to challenge: %s", filePath, challenge.Name)

	// Use the challenge-specific mutex to prevent race conditions during update checks
	challengeMutex := w.getChallengeUpdateMutex(challenge.Name)
	challengeMutex.Lock()

	// Check if this challenge is already being updated (within the mutex)
	if w.isUpdating(challenge.Name) {
		log.InfoH3("Challenge %s is already being updated, setting as pending", challenge.Name)
		w.setPendingUpdate(challenge.Name, filePath)
		challengeMutex.Unlock()
		return
	}

	// Mark as updating before releasing the mutex to prevent race conditions
	w.setUpdating(challenge.Name, true)
	challengeMutex.Unlock()

	// Start the update process
	w.processUpdate(challenge.Name, filePath)
}

// processUpdate handles the actual update process and checks for pending updates
func (w *Watcher) processUpdate(challengeName, filePath string) {
	// Note: updating flag is already set in handleFileChange
	defer w.setUpdating(challengeName, false)

	for {
		// Get the challenge info again (in case it changed)
		challenge, err := w.findChallengeByName(challengeName)
		if err != nil {
			log.Error("Failed to find challenge %s: %v", challengeName, err)
			return
		}

		if challenge == nil {
			log.Error("Challenge %s not found", challengeName)
			return
		}

		log.InfoH3("Processing update for challenge %s with file: %s", challengeName, filePath)

		// Get the per-challenge mutex to prevent race conditions
		challengeMutex := w.getChallengeUpdateMutex(challengeName)
		challengeMutex.Lock()

		// Determine what type of update is needed
		updateType := w.determineUpdateType(filePath, *challenge)

		// Perform the update
		switch updateType {
		case UpdateNone:
			log.Info("No update needed for challenge: %s", challengeName)

		case UpdateAttachment:
			log.Info("Updating attachment for challenge: %s", challengeName)
			if err := w.updateAttachmentOnly(*challenge); err != nil {
				log.Error("Failed to update attachment for challenge %s: %v", challengeName, err)
			} else {
				log.Info("Successfully updated attachment for challenge: %s", challengeName)
			}

		case UpdateMetadata:
			log.Info("Updating metadata and attachment for challenge: %s", challengeName)
			if err := w.updateMetadataAndAttachment(*challenge); err != nil {
				log.Error("Failed to update metadata for challenge %s: %v", challengeName, err)
			} else {
				log.Info("Successfully updated metadata for challenge: %s", challengeName)
			}

		case UpdateFullRedeploy:
			log.Info("Full redeployment needed for challenge: %s", challengeName)
			if err := w.fullRedeployChallenge(*challenge); err != nil {
				log.Error("Failed to redeploy challenge %s: %v", challengeName, err)
			} else {
				log.Info("Successfully redeployed challenge: %s", challengeName)
			}
		}

		challengeMutex.Unlock()

		// Check if there's a pending update
		if pendingFilePath, hasPending := w.getPendingUpdate(challengeName); hasPending {
			log.InfoH3("Found pending update for %s, processing: %s", challengeName, pendingFilePath)
			filePath = pendingFilePath // Update with the latest file path
			continue                   // Process the pending update
		}

		// No more pending updates, we're done
		break
	}

	log.InfoH3("Finished processing all updates for challenge: %s", challengeName)
}

// findChallengeForFile finds which challenge a file belongs to
func (w *Watcher) findChallengeForFile(filePath string) (*ChallengeYaml, error) {
	challenges, err := w.getChallenges()
	if err != nil {
		return nil, err
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}

	log.DebugH3("Looking for challenge that contains file: %s", absFilePath)
	log.DebugH3("Available challenges:")
	for _, ch := range challenges {
		log.DebugH3("  - %s: %s", ch.Name, ch.Cwd)
	}

	var bestMatch *ChallengeYaml
	var longestMatch int

	for _, challenge := range challenges {
		absChallengeDir, err := filepath.Abs(challenge.Cwd)
		if err != nil {
			log.DebugH3("Failed to get absolute path for challenge %s: %v", challenge.Name, err)
			continue
		}

		// Ensure the challenge directory path ends with a separator to avoid partial matches
		if !strings.HasSuffix(absChallengeDir, string(filepath.Separator)) {
			absChallengeDir += string(filepath.Separator)
		}

		log.DebugH3("Comparing:")
		log.DebugH3("  File path:       %s", absFilePath)
		log.DebugH3("  Challenge dir:   %s", absChallengeDir)
		log.DebugH3("  Challenge name:  %s", challenge.Name)

		if strings.HasPrefix(absFilePath, absChallengeDir) {
			// Found a match, but check if it's more specific than previous matches
			matchLength := len(absChallengeDir)
			log.DebugH3("  Match found! Length: %d", matchLength)

			if matchLength > longestMatch {
				longestMatch = matchLength
				bestMatch = &challenge
				log.DebugH3("  → New best match: %s (length: %d)", challenge.Name, matchLength)
			} else {
				log.DebugH3("  → Match found but not better than current best (length: %d vs %d)", matchLength, longestMatch)
			}
		} else {
			log.DebugH3("  → No match")
		}
	}

	if bestMatch != nil {
		log.DebugH3("Final result: Best matching challenge is %s", bestMatch.Name)
		return bestMatch, nil
	}

	log.DebugH3("Final result: No matching challenge found for file: %s", absFilePath)
	return nil, nil
}

// getChallenges gets the list of challenges to watch
func (w *Watcher) getChallenges() ([]ChallengeYaml, error) {
	appSettings, err := getAppSettings()
	if err != nil {
		return nil, err
	}

	config := &Config{appsettings: appSettings}
	return GetChallengesYaml(config)
}

// IsWatching returns true if the watcher is currently active
func (w *Watcher) IsWatching() bool {
	select {
	case <-w.ctx.Done():
		return false
	default:
		return true
	}
}

// GetWatchedChallenges returns the list of currently watched challenge directories
func (w *Watcher) GetWatchedChallenges() []string {
	challenges, err := w.getChallenges()
	if err != nil {
		log.Error("Failed to get challenges: %v", err)
		return []string{}
	}

	var dirs []string
	for _, challenge := range challenges {
		dirs = append(dirs, challenge.Cwd)
	}
	return dirs
}

// updateAttachmentOnly updates only the attachment for a challenge
func (w *Watcher) updateAttachmentOnly(challenge ChallengeYaml) error {
	log.InfoH2("Updating attachment only for challenge: %s", challenge.Name)

	// Get config and API setup
	config, err := GetConfig(w.gz.api)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	config.Event.CS = w.gz.api

	// Get existing challenge from API
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return fmt.Errorf("failed to get API challenges: %w", err)
	}

	// Find the challenge in API
	var challengeData *gzapi.Challenge
	for _, apiChallenge := range challenges {
		if apiChallenge.Title == challenge.Name {
			challengeData = &apiChallenge
			break
		}
	}

	if challengeData == nil {
		return fmt.Errorf("challenge %s not found in API", challenge.Name)
	}

	// Update only the attachment
	challengeData.CS = w.gz.api
	if err := handleChallengeAttachments(challenge, challengeData, w.gz.api); err != nil {
		return fmt.Errorf("failed to update attachment: %w", err)
	}

	return nil
}

// updateMetadataAndAttachment updates challenge metadata and attachment
func (w *Watcher) updateMetadataAndAttachment(challenge ChallengeYaml) error {
	log.InfoH2("Updating metadata and attachment for challenge: %s", challenge.Name)

	// Get config and API setup
	config, err := GetConfig(w.gz.api)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	config.Event.CS = w.gz.api

	// Get existing challenges from API
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return fmt.Errorf("failed to get API challenges: %w", err)
	}

	// Sync the challenge (this handles metadata and attachment updates)
	if err := syncChallenge(config, challenge, challenges, w.gz.api); err != nil {
		return fmt.Errorf("failed to sync challenge: %w", err)
	}

	return nil
}

// fullRedeployChallenge performs a full redeploy with stop and start scripts
func (w *Watcher) fullRedeployChallenge(challenge ChallengeYaml) error {
	log.InfoH2("Starting full redeployment of challenge: %s", challenge.Name)

	// Run stop script if exists
	if scriptValue, exists := challenge.Scripts["stop"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running stop script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "stop"); err != nil {
			log.Error("Stop script failed for %s: %v", challenge.Name, err)
			// Continue anyway - maybe the service wasn't running
		}
	}

	// Update metadata and attachment
	if err := w.updateMetadataAndAttachment(challenge); err != nil {
		return fmt.Errorf("failed to update challenge metadata: %w", err)
	}

	// Run start script if exists
	if scriptValue, exists := challenge.Scripts["start"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running start script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "start"); err != nil {
			return fmt.Errorf("start script failed: %w", err)
		}
	}

	// Run restart script if exists (typically for maintenance/monitoring)
	if scriptValue, exists := challenge.Scripts["restart"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running restart script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "restart"); err != nil {
			log.Error("Restart script failed for %s: %v", challenge.Name, err)
			// Don't return error for restart script failure - it's not critical for deployment
		}
	}

	return nil
}

// isUpdating checks if a challenge is currently being updated
func (w *Watcher) isUpdating(challengeName string) bool {
	w.updatingMu.RLock()
	defer w.updatingMu.RUnlock()
	return w.updatingChallenges[challengeName]
}

// setUpdating marks a challenge as updating or not updating
func (w *Watcher) setUpdating(challengeName string, updating bool) {
	w.updatingMu.Lock()
	defer w.updatingMu.Unlock()
	if updating {
		w.updatingChallenges[challengeName] = true
	} else {
		delete(w.updatingChallenges, challengeName)
	}
}

// setPendingUpdate sets the latest pending update for a challenge
func (w *Watcher) setPendingUpdate(challengeName, filePath string) {
	w.pendingUpdatesMu.Lock()
	defer w.pendingUpdatesMu.Unlock()
	w.pendingUpdates[challengeName] = filePath
	log.DebugH3("Set pending update for %s: %s", challengeName, filePath)
}

// getPendingUpdate gets and clears the pending update for a challenge
func (w *Watcher) getPendingUpdate(challengeName string) (string, bool) {
	w.pendingUpdatesMu.Lock()
	defer w.pendingUpdatesMu.Unlock()
	filePath, exists := w.pendingUpdates[challengeName]
	if exists {
		delete(w.pendingUpdates, challengeName)
		log.DebugH3("Retrieved pending update for %s: %s", challengeName, filePath)
	}
	return filePath, exists
}

// startIntervalScript starts an interval script for a challenge with proper tracking and validation
func (w *Watcher) startIntervalScript(challengeName, scriptName string, challenge ChallengeYaml, command string, interval time.Duration) {
	// Validate interval before starting
	if !validateInterval(interval, scriptName) {
		log.Error("Invalid interval for script '%s' in challenge '%s', skipping", scriptName, challengeName)
		return
	}

	w.intervalScriptsMu.Lock()
	defer w.intervalScriptsMu.Unlock()

	// Initialize map for challenge if it doesn't exist
	if w.intervalScripts[challengeName] == nil {
		w.intervalScripts[challengeName] = make(map[string]context.CancelFunc)
	}

	// Stop existing interval script if running
	if cancel, exists := w.intervalScripts[challengeName][scriptName]; exists {
		log.InfoH3("Stopping existing interval script '%s' for challenge '%s'", scriptName, challengeName)
		cancel()
	}

	// Initialize metrics if needed
	w.scriptMetricsMu.Lock()
	if w.scriptMetrics[challengeName] == nil {
		w.scriptMetrics[challengeName] = make(map[string]*ScriptMetrics)
	}
	if w.scriptMetrics[challengeName][scriptName] == nil {
		w.scriptMetrics[challengeName][scriptName] = &ScriptMetrics{
			IsInterval: true,
			Interval:   interval,
		}
	} else {
		// Update existing metrics with interval info
		w.scriptMetrics[challengeName][scriptName].IsInterval = true
		w.scriptMetrics[challengeName][scriptName].Interval = interval
	}
	w.scriptMetricsMu.Unlock()

	// Create new context for this interval script
	ctx, cancel := context.WithCancel(w.ctx)
	w.intervalScripts[challengeName][scriptName] = cancel

	// Start the interval script in a goroutine with watcher-specific logging
	go w.runWatcherIntervalScript(ctx, challengeName, scriptName, command, interval, challenge.Cwd)
}

// runWatcherIntervalScript runs an interval script with proper watcher integration and database logging
func (w *Watcher) runWatcherIntervalScript(ctx context.Context, challengeName, scriptName, command string, interval time.Duration, cwd string) {
	// Validate interval
	if !validateInterval(interval, scriptName) {
		log.Error("Invalid interval for script '%s' in challenge '%s', skipping", scriptName, challengeName)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.InfoH3("Started interval script '%s' for challenge '%s' with interval %v", scriptName, challengeName, interval)

	// Log initial start to database
	w.logScriptExecution(challengeName, scriptName, "interval", command, "started", 0, "", "", 0)

	for {
		select {
		case <-ctx.Done():
			log.InfoH3("Stopped interval script '%s' for challenge '%s' (context cancelled)", scriptName, challengeName)
			w.logScriptExecution(challengeName, scriptName, "interval", command, "stopped", 0, "", "Context cancelled", 0)
			return
		case <-ticker.C:
			log.InfoH3("Executing interval script '%s' for challenge '%s'", scriptName, challengeName)

			// Log execution start
			start := time.Now()
			w.logScriptExecution(challengeName, scriptName, "interval", command, "executing", 0, "", "", 0)

			// Update metrics
			w.scriptMetricsMu.Lock()
			if w.scriptMetrics[challengeName] != nil && w.scriptMetrics[challengeName][scriptName] != nil {
				w.scriptMetrics[challengeName][scriptName].LastExecution = start
				w.scriptMetrics[challengeName][scriptName].ExecutionCount++
			}
			w.scriptMetricsMu.Unlock()

			// Execute the script with context-aware execution and proper timeout
			var exitCode int = 0
			var success bool = false
			var errorOutput string = ""
			var output string = ""

			err := runShellForInterval(ctx, command, cwd, DefaultScriptTimeout)
			duration := time.Since(start)

			if err != nil {
				log.Error("Interval script '%s' failed for challenge '%s' after %v: %v", scriptName, challengeName, duration, err)
				success = false
				exitCode = 1
				errorOutput = err.Error()

				// Update metrics with error
				w.scriptMetricsMu.Lock()
				if w.scriptMetrics[challengeName] != nil && w.scriptMetrics[challengeName][scriptName] != nil {
					metrics := w.scriptMetrics[challengeName][scriptName]
					metrics.LastError = err
					metrics.LastDuration = duration
					metrics.TotalDuration += duration
				}
				w.scriptMetricsMu.Unlock()
			} else {
				log.InfoH3("Interval script '%s' completed successfully for challenge '%s' in %v", scriptName, challengeName, duration)
				success = true
				exitCode = 0

				// Update metrics with success
				w.scriptMetricsMu.Lock()
				if w.scriptMetrics[challengeName] != nil && w.scriptMetrics[challengeName][scriptName] != nil {
					metrics := w.scriptMetrics[challengeName][scriptName]
					metrics.LastError = nil
					metrics.LastDuration = duration
					metrics.TotalDuration += duration
				}
				w.scriptMetricsMu.Unlock()
			}

			// Log execution completion to database
			status := "failed"
			if success {
				status = "completed"
			}

			w.logScriptExecution(challengeName, scriptName, "interval", command, status,
				duration.Nanoseconds(), output, errorOutput, exitCode)
		}
	}
}

// stopIntervalScript stops a specific interval script for a challenge
func (w *Watcher) stopIntervalScript(challengeName, scriptName string) {
	w.intervalScriptsMu.Lock()
	defer w.intervalScriptsMu.Unlock()

	if challengeScripts, exists := w.intervalScripts[challengeName]; exists {
		if cancel, exists := challengeScripts[scriptName]; exists {
			log.InfoH3("Stopping interval script '%s' for challenge '%s'", scriptName, challengeName)
			cancel()
			delete(challengeScripts, scriptName)

			// Clean up empty challenge map
			if len(challengeScripts) == 0 {
				delete(w.intervalScripts, challengeName)
			}
		}
	}
}

// stopAllIntervalScripts stops all interval scripts for a challenge
func (w *Watcher) stopAllIntervalScripts(challengeName string) {
	w.intervalScriptsMu.Lock()
	defer w.intervalScriptsMu.Unlock()

	if challengeScripts, exists := w.intervalScripts[challengeName]; exists {
		log.InfoH3("Stopping all interval scripts for challenge '%s'", challengeName)
		for scriptName, cancel := range challengeScripts {
			log.InfoH3("  - Stopping interval script '%s'", scriptName)
			cancel()
		}
		delete(w.intervalScripts, challengeName)
	}
}

// runScriptWithIntervalSupport runs a script with proper interval script lifecycle management
func (w *Watcher) runScriptWithIntervalSupport(challengeConf ChallengeYaml, script string) error {
	scriptValue, exists := challengeConf.Scripts[script]
	if !exists {
		return nil
	}

	command := scriptValue.GetCommand()
	if command == "" {
		return nil
	}

	// Check if script has an interval configured
	if scriptValue.HasInterval() {
		interval := scriptValue.GetInterval()
		log.InfoH2("Starting interval script '%s' with interval %v", script, interval)
		log.InfoH3("Script command: %s", command)

		// Log script start
		w.logToDatabase("INFO", "script", challengeConf.Name, script,
			fmt.Sprintf("Starting interval script with interval %v", interval), "", 0)

		// Use watcher's interval script management
		w.startIntervalScript(challengeConf.Name, script, challengeConf, command, interval)
		return nil
	}

	// For non-interval scripts, stop any existing interval script with the same name
	w.stopIntervalScript(challengeConf.Name, script)

	// Initialize metrics for one-time script if needed
	w.scriptMetricsMu.Lock()
	if w.scriptMetrics[challengeConf.Name] == nil {
		w.scriptMetrics[challengeConf.Name] = make(map[string]*ScriptMetrics)
	}
	if w.scriptMetrics[challengeConf.Name][script] == nil {
		w.scriptMetrics[challengeConf.Name][script] = &ScriptMetrics{
			IsInterval: false,
			Interval:   0,
		}
	} else {
		// Update existing metrics to mark as non-interval
		w.scriptMetrics[challengeConf.Name][script].IsInterval = false
		w.scriptMetrics[challengeConf.Name][script].Interval = 0
	}
	w.scriptMetricsMu.Unlock()

	// Log script execution start
	start := time.Now()
	w.logScriptExecution(challengeConf.Name, script, "one-time", command, "started", 0, "", "", 0)

	// Run simple one-time script with timeout protection
	log.InfoH2("Running:\n%s", command)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultScriptTimeout)
	defer cancel()

	err := runShellWithContext(ctx, command, challengeConf.Cwd)
	duration := time.Since(start)

	// Update metrics
	w.scriptMetricsMu.Lock()
	if w.scriptMetrics[challengeConf.Name] != nil && w.scriptMetrics[challengeConf.Name][script] != nil {
		metrics := w.scriptMetrics[challengeConf.Name][script]
		metrics.LastExecution = start
		metrics.ExecutionCount++
		metrics.LastDuration = duration
		metrics.TotalDuration += duration
		if err != nil {
			metrics.LastError = err
		} else {
			metrics.LastError = nil
		}
	}
	w.scriptMetricsMu.Unlock()

	// Log script completion
	if err != nil {
		w.logToDatabase("ERROR", "script", challengeConf.Name, script,
			"One-time script execution failed", err.Error(), duration.Milliseconds())
		w.logScriptExecution(challengeConf.Name, script, "one-time", command, "failed", duration.Nanoseconds(), "", err.Error(), 1)
	} else {
		w.logToDatabase("INFO", "script", challengeConf.Name, script,
			"One-time script execution completed successfully", "", duration.Milliseconds())
		w.logScriptExecution(challengeConf.Name, script, "one-time", command, "completed", duration.Nanoseconds(), "", "", 0)
	}

	return err
}

// GetScriptMetrics returns script execution metrics for monitoring
func (w *Watcher) GetScriptMetrics() map[string]map[string]*ScriptMetrics {
	w.scriptMetricsMu.RLock()
	w.challengeConfigsMu.RLock()
	defer w.scriptMetricsMu.RUnlock()
	defer w.challengeConfigsMu.RUnlock()

	// Create a copy to avoid concurrent map access and enrich with interval information
	result := make(map[string]map[string]*ScriptMetrics)

	for challengeName, challengeMetrics := range w.scriptMetrics {
		result[challengeName] = make(map[string]*ScriptMetrics)

		// Get challenge config for interval information
		challengeConfig, hasConfig := w.challengeConfigs[challengeName]

		for scriptName, metrics := range challengeMetrics {
			// Create a copy of the metrics
			metricsCopy := &ScriptMetrics{
				LastExecution:  metrics.LastExecution,
				ExecutionCount: metrics.ExecutionCount,
				LastError:      metrics.LastError,
				LastDuration:   metrics.LastDuration,
				TotalDuration:  metrics.TotalDuration,
				IsInterval:     false,
				Interval:       0,
			}

			// Check if this script has interval configuration
			if hasConfig {
				if scriptValue, exists := challengeConfig.Scripts[scriptName]; exists {
					if scriptValue.HasInterval() {
						metricsCopy.IsInterval = true
						metricsCopy.Interval = scriptValue.GetInterval()
					}
				}
			}

			result[challengeName][scriptName] = metricsCopy
		}
	}
	return result
}

// GetActiveIntervalScripts returns a list of currently running interval scripts
func (w *Watcher) GetActiveIntervalScripts() map[string][]string {
	w.intervalScriptsMu.RLock()
	defer w.intervalScriptsMu.RUnlock()

	result := make(map[string][]string)
	for challengeName, scripts := range w.intervalScripts {
		result[challengeName] = make([]string, 0, len(scripts))
		for scriptName := range scripts {
			result[challengeName] = append(result[challengeName], scriptName)
		}
	}
	return result
}

// Database initialization and management functions
func (w *Watcher) initDatabase() error {
	if !w.config.DatabaseEnabled {
		log.Info("Database logging disabled")
		return nil
	}

	dbPath := w.config.DatabasePath
	if dbPath == "" {
		dbPath = DefaultWatcherConfig.DatabasePath
	}

	log.Info("Initializing SQLite database: %s", dbPath)

	// Create database directory if it doesn't exist
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	w.dbMu.Lock()
	w.db = db
	w.dbMu.Unlock()

	// Create tables
	if err := w.createDatabaseTables(); err != nil {
		return fmt.Errorf("failed to create database tables: %w", err)
	}

	log.Info("Database initialized successfully")
	return nil
}

func (w *Watcher) createDatabaseTables() error {
	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Create watcher_logs table
	createLogsTable := `
		CREATE TABLE IF NOT EXISTS watcher_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			level TEXT NOT NULL,
			component TEXT NOT NULL,
			challenge TEXT,
			script TEXT,
			message TEXT NOT NULL,
			error TEXT,
			duration INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON watcher_logs(timestamp);
		CREATE INDEX IF NOT EXISTS idx_logs_level ON watcher_logs(level);
		CREATE INDEX IF NOT EXISTS idx_logs_challenge ON watcher_logs(challenge);
	`

	// Create challenge_states table
	createStatesTable := `
		CREATE TABLE IF NOT EXISTS challenge_states (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			challenge_name TEXT UNIQUE NOT NULL,
			status TEXT NOT NULL,
			last_update DATETIME DEFAULT CURRENT_TIMESTAMP,
			error_message TEXT,
			script_states TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_states_name ON challenge_states(challenge_name);
		CREATE INDEX IF NOT EXISTS idx_states_status ON challenge_states(status);
	`

	// Create script_executions table
	createExecutionsTable := `
		CREATE TABLE IF NOT EXISTS script_executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			challenge_name TEXT NOT NULL,
			script_name TEXT NOT NULL,
			script_type TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT NOT NULL,
			duration INTEGER,
			output TEXT,
			error_output TEXT,
			exit_code INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_executions_timestamp ON script_executions(timestamp);
		CREATE INDEX IF NOT EXISTS idx_executions_challenge ON script_executions(challenge_name);
		CREATE INDEX IF NOT EXISTS idx_executions_script ON script_executions(script_name);
		CREATE INDEX IF NOT EXISTS idx_executions_status ON script_executions(status);
	`

	// Execute table creation statements
	if _, err := db.Exec(createLogsTable); err != nil {
		return fmt.Errorf("failed to create watcher_logs table: %w", err)
	}

	if _, err := db.Exec(createStatesTable); err != nil {
		return fmt.Errorf("failed to create challenge_states table: %w", err)
	}

	if _, err := db.Exec(createExecutionsTable); err != nil {
		return fmt.Errorf("failed to create script_executions table: %w", err)
	}

	log.Info("Database tables created successfully")
	return nil
}

func (w *Watcher) closeDatabase() error {
	w.dbMu.Lock()
	defer w.dbMu.Unlock()

	if w.db != nil {
		log.Info("Closing database connection")
		err := w.db.Close()
		w.db = nil
		return err
	}
	return nil
}

// Database logging functions
func (w *Watcher) logToDatabase(level, component, challenge, script, message, errorMsg string, duration int64) {
	if !w.config.DatabaseEnabled {
		return
	}

	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return
	}

	query := `
		INSERT INTO watcher_logs (level, component, challenge, script, message, error, duration)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query, level, component, challenge, script, message, errorMsg, duration)
	if err != nil {
		// Don't use log.Error here to avoid potential recursion
		fmt.Printf("Failed to log to database: %v\n", err)
	}
}

func (w *Watcher) updateChallengeState(challengeName, status, errorMessage string) {
	if !w.config.DatabaseEnabled {
		return
	}

	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return
	}

	// Get current script states
	activeScripts := w.GetActiveIntervalScripts()
	scriptStatesJSON, _ := json.Marshal(activeScripts[challengeName])

	query := `
		INSERT OR REPLACE INTO challenge_states (challenge_name, status, last_update, error_message, script_states)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?)
	`

	_, err := db.Exec(query, challengeName, status, errorMessage, string(scriptStatesJSON))
	if err != nil {
		fmt.Printf("Failed to update challenge state: %v\n", err)
	}
}

func (w *Watcher) logScriptExecution(challengeName, scriptName, scriptType, command, status string, duration int64, output, errorOutput string, exitCode int) {
	if !w.config.DatabaseEnabled {
		return
	}

	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return
	}

	query := `
		INSERT INTO script_executions (challenge_name, script_name, script_type, command, status, duration, output, error_output, exit_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query, challengeName, scriptName, scriptType, command, status, duration, output, errorOutput, exitCode)
	if err != nil {
		fmt.Printf("Failed to log script execution: %v\n", err)
	}
}

// Socket server initialization and management functions
func (w *Watcher) initSocketServer() error {
	if !w.config.SocketEnabled {
		log.Info("Socket server disabled")
		return nil
	}

	socketPath := w.config.SocketPath
	if socketPath == "" {
		socketPath = DefaultWatcherConfig.SocketPath
	}

	// Remove existing socket file if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Error("Failed to remove existing socket file: %v", err)
	}

	// Create socket directory if it doesn't exist
	socketDir := filepath.Dir(socketPath)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Create Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	w.socketMu.Lock()
	w.socketServer = listener
	w.socketPath = socketPath
	w.socketMu.Unlock()

	log.Info("Socket server initialized: %s", socketPath)
	return nil
}

func (w *Watcher) closeSocketServer() error {
	w.socketMu.Lock()
	defer w.socketMu.Unlock()

	if w.socketServer != nil {
		log.Info("Closing socket server")
		err := w.socketServer.Close()
		w.socketServer = nil

		// Clean up socket file
		if w.socketPath != "" {
			if removeErr := os.Remove(w.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Error("Failed to remove socket file: %v", removeErr)
			}
			w.socketPath = ""
		}
		return err
	}
	return nil
}

func (w *Watcher) socketServerLoop() {
	w.socketMu.RLock()
	listener := w.socketServer
	w.socketMu.RUnlock()

	if listener == nil {
		return
	}

	log.Info("Starting socket server loop")

	for {
		select {
		case <-w.ctx.Done():
			log.Info("Socket server loop stopped")
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				// Check if we're shutting down
				select {
				case <-w.ctx.Done():
					return
				default:
					log.Error("Failed to accept socket connection: %v", err)
					continue
				}
			}

			// Handle connection in goroutine
			go w.handleSocketConnection(conn)
		}
	}
}

func (w *Watcher) handleSocketConnection(conn net.Conn) {
	defer conn.Close()

	// Set connection timeout
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var cmd WatcherCommand
	if err := decoder.Decode(&cmd); err != nil {
		response := WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to decode command: %v", err),
		}
		encoder.Encode(response)
		return
	}

	// Process command
	response := w.processSocketCommand(cmd)

	// Send response
	if err := encoder.Encode(response); err != nil {
		log.Error("Failed to send socket response: %v", err)
	}
}

func (w *Watcher) processSocketCommand(cmd WatcherCommand) WatcherResponse {
	switch cmd.Action {
	case "status":
		return w.handleStatusCommand(cmd)
	case "list_challenges":
		return w.handleListChallengesCommand(cmd)
	case "get_metrics":
		return w.handleGetMetricsCommand(cmd)
	case "get_logs":
		return w.handleGetLogsCommand(cmd)
	case "stop_script":
		return w.handleStopScriptCommand(cmd)
	case "restart_challenge":
		return w.handleRestartChallengeCommand(cmd)
	case "get_script_executions":
		return w.handleGetScriptExecutionsCommand(cmd)
	default:
		return WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Unknown command: %s", cmd.Action),
		}
	}
}

// Socket command handlers
func (w *Watcher) handleStatusCommand(cmd WatcherCommand) WatcherResponse {
	activeScripts := w.GetActiveIntervalScripts()
	challenges := len(w.watchedChallenges)

	status := map[string]interface{}{
		"status":             "running",
		"watched_challenges": challenges,
		"active_scripts":     activeScripts,
		"database_enabled":   w.config.DatabaseEnabled,
		"socket_enabled":     w.config.SocketEnabled,
		"uptime":             time.Since(time.Now()).String(), // You might want to track actual startup time
	}

	return WatcherResponse{
		Success: true,
		Message: "Watcher status retrieved successfully",
		Data:    status,
	}
}

func (w *Watcher) handleListChallengesCommand(cmd WatcherCommand) WatcherResponse {
	w.watchedMu.RLock()
	challenges := make([]map[string]interface{}, 0, len(w.watchedChallenges))
	for name, watching := range w.watchedChallenges {
		challengeInfo := map[string]interface{}{
			"name":     name,
			"watching": watching,
		}
		if dir, exists := w.watchedChallengeDirs[name]; exists {
			challengeInfo["directory"] = dir
		}
		challenges = append(challenges, challengeInfo)
	}
	w.watchedMu.RUnlock()

	return WatcherResponse{
		Success: true,
		Message: fmt.Sprintf("Found %d challenges", len(challenges)),
		Data:    map[string]interface{}{"challenges": challenges},
	}
}

func (w *Watcher) handleGetMetricsCommand(cmd WatcherCommand) WatcherResponse {
	metrics := w.GetScriptMetrics()

	return WatcherResponse{
		Success: true,
		Message: "Script metrics retrieved successfully",
		Data:    map[string]interface{}{"metrics": metrics},
	}
}

func (w *Watcher) handleGetLogsCommand(cmd WatcherCommand) WatcherResponse {
	if !w.config.DatabaseEnabled {
		return WatcherResponse{
			Success: false,
			Error:   "Database logging is disabled",
		}
	}

	// Get limit from command data (default to 100)
	limit := 100
	if cmd.Data != nil {
		if l, ok := cmd.Data["limit"].(float64); ok {
			limit = int(l)
		}
	}

	logs, err := w.getRecentLogs(limit)
	if err != nil {
		return WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to get logs: %v", err),
		}
	}

	return WatcherResponse{
		Success: true,
		Message: fmt.Sprintf("Retrieved %d log entries", len(logs)),
		Data:    map[string]interface{}{"logs": logs},
	}
}

func (w *Watcher) handleStopScriptCommand(cmd WatcherCommand) WatcherResponse {
	if cmd.Data == nil {
		return WatcherResponse{
			Success: false,
			Error:   "Missing challenge_name and script_name parameters",
		}
	}

	challengeName, ok1 := cmd.Data["challenge_name"].(string)
	scriptName, ok2 := cmd.Data["script_name"].(string)

	if !ok1 || !ok2 {
		return WatcherResponse{
			Success: false,
			Error:   "Invalid challenge_name or script_name parameter",
		}
	}

	w.stopIntervalScript(challengeName, scriptName)

	return WatcherResponse{
		Success: true,
		Message: fmt.Sprintf("Stopped script '%s' for challenge '%s'", scriptName, challengeName),
	}
}

func (w *Watcher) handleRestartChallengeCommand(cmd WatcherCommand) WatcherResponse {
	if cmd.Data == nil {
		return WatcherResponse{
			Success: false,
			Error:   "Missing challenge_name parameter",
		}
	}

	challengeName, ok := cmd.Data["challenge_name"].(string)
	if !ok {
		return WatcherResponse{
			Success: false,
			Error:   "Invalid challenge_name parameter",
		}
	}

	// Find challenge
	challenge, err := w.findChallengeByName(challengeName)
	if err != nil {
		return WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to find challenge: %v", err),
		}
	}

	if challenge == nil {
		return WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Challenge '%s' not found", challengeName),
		}
	}

	// Trigger full redeploy in background
	go func() {
		w.updateChallengeState(challengeName, "restarting", "")
		if err := w.fullRedeployChallenge(*challenge); err != nil {
			w.updateChallengeState(challengeName, "error", err.Error())
			log.Error("Failed to restart challenge %s: %v", challengeName, err)
		} else {
			w.updateChallengeState(challengeName, "watching", "")
		}
	}()

	return WatcherResponse{
		Success: true,
		Message: fmt.Sprintf("Challenge '%s' restart initiated", challengeName),
	}
}

func (w *Watcher) handleGetScriptExecutionsCommand(cmd WatcherCommand) WatcherResponse {
	if !w.config.DatabaseEnabled {
		return WatcherResponse{
			Success: false,
			Error:   "Database logging is disabled",
		}
	}

	limit := 100
	challengeName := ""

	if cmd.Data != nil {
		if l, ok := cmd.Data["limit"].(float64); ok {
			limit = int(l)
		}
		if c, ok := cmd.Data["challenge_name"].(string); ok {
			challengeName = c
		}
	}

	executions, err := w.getScriptExecutions(challengeName, limit)
	if err != nil {
		return WatcherResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to get script executions: %v", err),
		}
	}

	return WatcherResponse{
		Success: true,
		Message: fmt.Sprintf("Retrieved %d script executions", len(executions)),
		Data:    map[string]interface{}{"executions": executions},
	}
}

// Database query helper functions
func (w *Watcher) getRecentLogs(limit int) ([]WatcherLog, error) {
	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, timestamp, level, component, challenge, script, message, error, duration
		FROM watcher_logs
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []WatcherLog
	for rows.Next() {
		var log WatcherLog
		var challenge, script, errorMsg sql.NullString
		var duration sql.NullInt64

		err := rows.Scan(
			&log.ID, &log.Timestamp, &log.Level, &log.Component,
			&challenge, &script, &log.Message, &errorMsg, &duration,
		)
		if err != nil {
			return nil, err
		}

		log.Challenge = challenge.String
		log.Script = script.String
		log.Error = errorMsg.String
		log.Duration = duration.Int64

		logs = append(logs, log)
	}

	return logs, rows.Err()
}

func (w *Watcher) getScriptExecutions(challengeName string, limit int) ([]ScriptExecution, error) {
	w.dbMu.RLock()
	db := w.db
	w.dbMu.RUnlock()

	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var query string
	var args []interface{}

	if challengeName != "" {
		query = `
			SELECT id, timestamp, challenge_name, script_name, script_type, command, status, duration, output, error_output, exit_code
			FROM script_executions
			WHERE challenge_name = ?
			ORDER BY timestamp DESC
			LIMIT ?
		`
		args = []interface{}{challengeName, limit}
	} else {
		query = `
			SELECT id, timestamp, challenge_name, script_name, script_type, command, status, duration, output, error_output, exit_code
			FROM script_executions
			ORDER BY timestamp DESC
			LIMIT ?
		`
		args = []interface{}{limit}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []ScriptExecution
	for rows.Next() {
		var exec ScriptExecution
		var duration sql.NullInt64
		var output, errorOutput sql.NullString
		var exitCode sql.NullInt64

		err := rows.Scan(
			&exec.ID, &exec.Timestamp, &exec.ChallengeName, &exec.ScriptName,
			&exec.ScriptType, &exec.Command, &exec.Status, &duration,
			&output, &errorOutput, &exitCode,
		)
		if err != nil {
			return nil, err
		}

		exec.Duration = duration.Int64
		exec.Output = output.String
		exec.ErrorOutput = errorOutput.String
		exec.ExitCode = int(exitCode.Int64)

		// Compute success based on status and exit code
		exec.Success = (exec.Status == "completed" && exitCode.Valid && exitCode.Int64 == 0) ||
			(exec.Status == "completed" && !exitCode.Valid)

		executions = append(executions, exec)
	}

	return executions, rows.Err()
}

// WatcherClient provides a client interface to communicate with the watcher daemon
type WatcherClient struct {
	socketPath string
	timeout    time.Duration
}

// NewWatcherClient creates a new watcher client
func NewWatcherClient(socketPath string) *WatcherClient {
	if socketPath == "" {
		socketPath = DefaultWatcherConfig.SocketPath
	}
	return &WatcherClient{
		socketPath: socketPath,
		timeout:    30 * time.Second,
	}
}

// SetTimeout sets the connection timeout for the client
func (c *WatcherClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// SendCommand sends a command to the watcher and returns the response
func (c *WatcherClient) SendCommand(action string, data map[string]interface{}) (*WatcherResponse, error) {
	// Connect to the socket
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to watcher socket %s: %w", c.socketPath, err)
	}
	defer conn.Close()

	// Set read/write deadline
	deadline := time.Now().Add(c.timeout)
	conn.SetDeadline(deadline)

	// Create and send command
	cmd := WatcherCommand{
		Action: action,
		Data:   data,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(cmd); err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var response WatcherResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// Status gets the current watcher status
func (c *WatcherClient) Status() (*WatcherResponse, error) {
	return c.SendCommand("status", nil)
}

// ListChallenges gets the list of watched challenges
func (c *WatcherClient) ListChallenges() (*WatcherResponse, error) {
	return c.SendCommand("list_challenges", nil)
}

// GetMetrics gets script execution metrics
func (c *WatcherClient) GetMetrics() (*WatcherResponse, error) {
	return c.SendCommand("get_metrics", nil)
}

// GetLogs gets recent logs from the database
func (c *WatcherClient) GetLogs(limit int) (*WatcherResponse, error) {
	data := map[string]interface{}{
		"limit": limit,
	}
	return c.SendCommand("get_logs", data)
}

// StopScript stops a specific interval script
func (c *WatcherClient) StopScript(challengeName, scriptName string) (*WatcherResponse, error) {
	data := map[string]interface{}{
		"challenge_name": challengeName,
		"script_name":    scriptName,
	}
	return c.SendCommand("stop_script", data)
}

// RestartChallenge triggers a full restart of a challenge
func (c *WatcherClient) RestartChallenge(challengeName string) (*WatcherResponse, error) {
	data := map[string]interface{}{
		"challenge_name": challengeName,
	}
	return c.SendCommand("restart_challenge", data)
}

// GetScriptExecutions gets script execution history
func (c *WatcherClient) GetScriptExecutions(challengeName string, limit int) (*WatcherResponse, error) {
	data := map[string]interface{}{
		"limit": limit,
	}
	if challengeName != "" {
		data["challenge_name"] = challengeName
	}
	return c.SendCommand("get_script_executions", data)
}

// StreamLiveLogs streams database logs in real-time
func (c *WatcherClient) StreamLiveLogs(limit int, interval time.Duration) error {
	fmt.Printf("📡 Live Database Logs (refreshing every %v)\n", interval)
	fmt.Println("==========================================")
	fmt.Println("Press Ctrl+C to stop streaming")
	fmt.Println()

	var lastLogID int64 = 0

	for {
		// Get recent logs
		response, err := c.GetLogs(limit)
		if err != nil {
			fmt.Printf("❌ Error getting logs: %v\n", err)
			time.Sleep(interval)
			continue
		}

		if !response.Success {
			fmt.Printf("❌ Failed to get logs: %s\n", response.Error)
			time.Sleep(interval)
			continue
		}

		// Process and display new logs
		if data, ok := response.Data["logs"].([]interface{}); ok && len(data) > 0 {
			newLogs := []interface{}{}

			// Filter for new logs only
			for _, logInterface := range data {
				if logMap, ok := logInterface.(map[string]interface{}); ok {
					if idFloat, ok := logMap["id"].(float64); ok {
						logID := int64(idFloat)
						if logID > lastLogID {
							newLogs = append(newLogs, logInterface)
							if logID > lastLogID {
								lastLogID = logID
							}
						}
					}
				}
			}

			// Display new logs (reverse order to show newest first)
			if len(newLogs) > 0 {
				for i := len(newLogs) - 1; i >= 0; i-- {
					logInterface := newLogs[i]
					if logMap, ok := logInterface.(map[string]interface{}); ok {
						c.displayLogEntry(logMap)
					}
				}
			}
		}

		time.Sleep(interval)
	}
}

// displayLogEntry formats and displays a single log entry
func (c *WatcherClient) displayLogEntry(logMap map[string]interface{}) {
	timestamp := ""
	if t, ok := logMap["timestamp"].(string); ok {
		if parsed, err := time.Parse("2006-01-02T15:04:05Z", t); err == nil {
			timestamp = parsed.Format("15:04:05")
		} else {
			timestamp = t
		}
	}

	level := ""
	if l, ok := logMap["level"].(string); ok {
		level = l
	}

	component := ""
	if comp, ok := logMap["component"].(string); ok {
		component = comp
	}

	challenge := ""
	if ch, ok := logMap["challenge"].(string); ok && ch != "" {
		challenge = fmt.Sprintf("[%s]", ch)
	}

	script := ""
	if sc, ok := logMap["script"].(string); ok && sc != "" {
		script = fmt.Sprintf("/%s", sc)
	}

	message := ""
	if m, ok := logMap["message"].(string); ok {
		message = m
	}

	levelIcon := "ℹ️"
	switch level {
	case "ERROR":
		levelIcon = "❌"
	case "WARN":
		levelIcon = "⚠️"
	case "INFO":
		levelIcon = "ℹ️"
	case "DEBUG":
		levelIcon = "🔍"
	}

	fmt.Printf("[%s] %s %s %s%s %s\n", timestamp, levelIcon, component, challenge, script, message)
}

// IsWatcherRunning checks if the watcher daemon is running
func (c *WatcherClient) IsWatcherRunning() bool {
	response, err := c.Status()
	return err == nil && response.Success
}

// WaitForWatcher waits for the watcher to become available
func (c *WatcherClient) WaitForWatcher(maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if c.IsWatcherRunning() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("watcher did not become available within %v", maxWait)
}

// PrintStatus prints a formatted status report
func (c *WatcherClient) PrintStatus() error {
	response, err := c.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("status request failed: %s", response.Error)
	}

	fmt.Println("🔍 GZCTF Watcher Status")
	fmt.Println("==========================================")

	if data, ok := response.Data["status"].(string); ok && data == "running" {
		fmt.Println("🟢 Status: RUNNING")
	} else {
		fmt.Println("🔴 Status: UNKNOWN")
	}

	if challenges, ok := response.Data["watched_challenges"].(float64); ok {
		fmt.Printf("📁 Watched Challenges: %.0f\n", challenges)
	}

	if dbEnabled, ok := response.Data["database_enabled"].(bool); ok {
		if dbEnabled {
			fmt.Println("🗄️  Database: ENABLED")
		} else {
			fmt.Println("🗄️  Database: DISABLED")
		}
	}

	if socketEnabled, ok := response.Data["socket_enabled"].(bool); ok {
		if socketEnabled {
			fmt.Println("🔌 Socket Server: ENABLED")
		} else {
			fmt.Println("🔌 Socket Server: DISABLED")
		}
	}

	if activeScripts, ok := response.Data["active_scripts"].(map[string]interface{}); ok && len(activeScripts) > 0 {
		fmt.Println("\n🔄 Active Interval Scripts:")
		for challengeName, scriptsInterface := range activeScripts {
			if scripts, ok := scriptsInterface.([]interface{}); ok && len(scripts) > 0 {
				fmt.Printf("  📦 %s:\n", challengeName)
				for _, scriptInterface := range scripts {
					if script, ok := scriptInterface.(string); ok {
						fmt.Printf("    - %s\n", script)
					}
				}
			}
		}
	}

	fmt.Println("\n🛠️  Available Commands:")
	fmt.Println("   gzcli watcher-client status")
	fmt.Println("   gzcli watcher-client list")
	fmt.Println("   gzcli watcher-client logs [--watcher-limit N]")
	fmt.Println("   gzcli watcher-client live-logs [--watcher-limit N] [--watcher-interval 2s]")
	fmt.Println("   gzcli watcher-client metrics")
	fmt.Println("   gzcli watcher-client executions [--watcher-challenge NAME]")
	fmt.Println("   gzcli watcher-client stop-script --watcher-challenge NAME --watcher-script SCRIPT")
	fmt.Println("   gzcli watcher-client restart --watcher-challenge NAME")

	return nil
}

// PrintChallenges prints a formatted list of challenges
func (c *WatcherClient) PrintChallenges() error {
	response, err := c.ListChallenges()
	if err != nil {
		return fmt.Errorf("failed to list challenges: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("list challenges request failed: %s", response.Error)
	}

	fmt.Println("📁 Watched Challenges")
	fmt.Println("==========================================")

	if data, ok := response.Data["challenges"].([]interface{}); ok {
		if len(data) == 0 {
			fmt.Println("No challenges are currently being watched.")
			return nil
		}

		for i, challengeInterface := range data {
			if challenge, ok := challengeInterface.(map[string]interface{}); ok {
				name := "unknown"
				if n, ok := challenge["name"].(string); ok {
					name = n
				}

				watching := false
				if w, ok := challenge["watching"].(bool); ok {
					watching = w
				}

				directory := ""
				if d, ok := challenge["directory"].(string); ok {
					directory = d
				}

				status := "🔴"
				if watching {
					status = "🟢"
				}

				fmt.Printf("%d. %s %s\n", i+1, status, name)
				if directory != "" {
					fmt.Printf("   📂 %s\n", directory)
				}
			}
		}
	}

	return nil
}

// PrintLogs prints formatted recent logs
func (c *WatcherClient) PrintLogs(limit int) error {
	response, err := c.GetLogs(limit)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("get logs request failed: %s", response.Error)
	}

	fmt.Printf("📋 Recent Logs (last %d entries)\n", limit)
	fmt.Println("==========================================")

	if data, ok := response.Data["logs"].([]interface{}); ok {
		if len(data) == 0 {
			fmt.Println("No logs available.")
			return nil
		}

		for _, logInterface := range data {
			if logMap, ok := logInterface.(map[string]interface{}); ok {
				timestamp := ""
				if t, ok := logMap["timestamp"].(string); ok {
					if parsed, err := time.Parse("2006-01-02T15:04:05Z", t); err == nil {
						timestamp = parsed.Format("15:04:05")
					} else {
						timestamp = t
					}
				}

				level := ""
				if l, ok := logMap["level"].(string); ok {
					level = l
				}

				component := ""
				if c, ok := logMap["component"].(string); ok {
					component = c
				}

				challenge := ""
				if ch, ok := logMap["challenge"].(string); ok && ch != "" {
					challenge = fmt.Sprintf("[%s]", ch)
				}

				message := ""
				if m, ok := logMap["message"].(string); ok {
					message = m
				}

				levelIcon := "ℹ️"
				switch level {
				case "ERROR":
					levelIcon = "❌"
				case "WARN":
					levelIcon = "⚠️"
				case "INFO":
					levelIcon = "ℹ️"
				case "DEBUG":
					levelIcon = "🔍"
				}

				fmt.Printf("[%s] %s %s %s %s\n", timestamp, levelIcon, component, challenge, message)
			}
		}
	}

	return nil
}

// PrintMetrics prints formatted script metrics
func (c *WatcherClient) PrintMetrics() error {
	response, err := c.GetMetrics()
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("get metrics request failed: %s", response.Error)
	}

	fmt.Println("📊 Script Execution Metrics")
	fmt.Println("==========================================")

	if data, ok := response.Data["metrics"].(map[string]interface{}); ok {
		if len(data) == 0 {
			fmt.Println("No metrics available.")
			return nil
		}

		for challengeName, challengeInterface := range data {
			if challengeMetrics, ok := challengeInterface.(map[string]interface{}); ok {
				fmt.Printf("\n📦 Challenge: %s\n", challengeName)
				fmt.Println("   Scripts:")

				for scriptName, scriptInterface := range challengeMetrics {
					if scriptMetrics, ok := scriptInterface.(map[string]interface{}); ok {
						execCount := float64(0)
						if ec, ok := scriptMetrics["ExecutionCount"].(float64); ok {
							execCount = ec
						}

						lastExecution := ""
						if le, ok := scriptMetrics["LastExecution"].(string); ok {
							if parsed, err := time.Parse("2006-01-02T15:04:05Z", le); err == nil {
								lastExecution = parsed.Format("2006-01-02 15:04:05")
							} else {
								lastExecution = le
							}
						}

						lastDuration := ""
						if ld, ok := scriptMetrics["LastDuration"].(float64); ok {
							if ld >= 1000000000 { // >= 1 second
								lastDuration = fmt.Sprintf("%.1fs", ld/1000000000)
							} else if ld >= 1000000 { // >= 1 millisecond
								lastDuration = fmt.Sprintf("%.0fms", ld/1000000)
							} else if ld > 0 {
								lastDuration = fmt.Sprintf("%.0fμs", ld/1000)
							}
						}

						// Check if this is an interval script
						isInterval := false
						if ii, ok := scriptMetrics["is_interval"].(bool); ok {
							isInterval = ii
						}

						interval := ""
						if isInterval {
							if iv, ok := scriptMetrics["interval"].(float64); ok && iv > 0 {
								intervalDuration := time.Duration(iv)
								if intervalDuration >= time.Hour {
									interval = fmt.Sprintf(" [interval: %.0fh]", intervalDuration.Hours())
								} else if intervalDuration >= time.Minute {
									interval = fmt.Sprintf(" [interval: %.0fm]", intervalDuration.Minutes())
								} else {
									interval = fmt.Sprintf(" [interval: %.0fs]", intervalDuration.Seconds())
								}
							}
						}

						// Create script type indicator
						scriptType := ""
						if isInterval {
							scriptType = "🔄 "
						} else {
							scriptType = "▶️ "
						}

						fmt.Printf("     %s%s: %.0f executions", scriptType, scriptName, execCount)
						if lastExecution != "" && lastExecution != "0001-01-01 00:00:00" {
							fmt.Printf(", last: %s", lastExecution)
						}
						if lastDuration != "" {
							fmt.Printf(" (%s)", lastDuration)
						}
						if interval != "" {
							fmt.Printf("%s", interval)
						}
						fmt.Println()
					}
				}
			}
		}
	}

	return nil
}

// newChallengeCheckLoop periodically checks for new challenges
func (w *Watcher) newChallengeCheckLoop(config WatcherConfig) {
	// Additional safeguard against zero duration
	interval := config.NewChallengeCheckInterval
	if interval <= 0 {
		interval = DefaultWatcherConfig.NewChallengeCheckInterval
		log.Error("NewChallengeCheckInterval was zero or negative in loop, using default: %v", interval)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Info("New challenge check loop stopped")
			return
		case <-ticker.C:
			w.checkForNewChallenges()
		}
	}
}

// checkForNewChallenges checks for new challenges and adds them to the watcher
func (w *Watcher) checkForNewChallenges() {
	log.DebugH3("Checking for new challenges...")

	challenges, err := w.getChallenges()
	if err != nil {
		log.Error("Failed to get challenges for new challenge check: %v", err)
		return
	}

	w.checkAndAddNewChallenges(challenges)
}

// checkAndAddNewChallenges checks the provided challenges list and adds any new ones to the watcher
func (w *Watcher) checkAndAddNewChallenges(challenges []ChallengeYaml) {
	newChallenges := []ChallengeYaml{}

	// Check which challenges are not being watched yet
	w.watchedMu.RLock()
	for _, challenge := range challenges {
		if !w.watchedChallenges[challenge.Name] {
			newChallenges = append(newChallenges, challenge)
		}
	}
	w.watchedMu.RUnlock()

	// Add new challenges to the watcher and sync them
	for _, challenge := range newChallenges {
		log.InfoH2("🆕 New challenge detected: %s", challenge.Name)

		// Add to watcher first
		if err := w.addChallengeToWatch(challenge); err != nil {
			log.Error("Failed to watch new challenge %s: %v", challenge.Name, err)
			w.logToDatabase("ERROR", "watcher", challenge.Name, "", "Failed to add new challenge to watcher", err.Error(), 0)
			continue
		}
		w.logToDatabase("INFO", "watcher", challenge.Name, "", "New challenge added to watcher successfully", "", 0)
		log.InfoH3("✅ Successfully added new challenge to watcher: %s", challenge.Name)

		// Sync and deploy the new challenge sequentially to prevent race conditions
		log.InfoH3("🔄 Syncing and deploying new challenge: %s", challenge.Name)
		go w.syncAndDeployNewChallenge(challenge)
	}

	if len(newChallenges) == 0 {
		log.DebugH3("No new challenges found")
	} else {
		log.InfoH2("🎯 Found %d new challenge(s) - added to watcher and syncing to platform", len(newChallenges))
	}
}

// syncAndDeployNewChallenge syncs and deploys a newly detected challenge sequentially to prevent race conditions
func (w *Watcher) syncAndDeployNewChallenge(challenge ChallengeYaml) {
	// Get a mutex for this challenge to prevent race conditions
	mutex := w.getChallengeUpdateMutex(challenge.Name)
	mutex.Lock()
	defer mutex.Unlock()

	log.InfoH2("🚀 Starting sync and deploy for new challenge: %s", challenge.Name)

	// Step 1: Sync the challenge to create it in the API
	config, err := GetConfig(w.gz.api)
	if err != nil {
		log.Error("❌ Failed to get config for new challenge sync %s: %v", challenge.Name, err)
		return
	}
	config.Event.CS = w.gz.api

	// Get existing challenges from API
	existingChallenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("❌ Failed to get API challenges for new challenge sync %s: %v", challenge.Name, err)
		return
	}

	// Sync the challenge (this will create it if it doesn't exist)
	if err := syncChallenge(config, challenge, existingChallenges, w.gz.api); err != nil {
		log.Error("❌ Failed to sync new challenge %s: %v", challenge.Name, err)
		return
	}

	log.InfoH2("✅ Successfully synced new challenge: %s", challenge.Name)

	// Step 2: Deploy the challenge (run start script if exists)
	log.InfoH2("🚀 Starting deployment for new challenge: %s", challenge.Name)

	// Run start script if exists
	if scriptValue, exists := challenge.Scripts["start"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running start script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "start"); err != nil {
			log.Error("❌ Start script failed for %s: %v", challenge.Name, err)
			return
		}
	}

	// Run restart script if exists (typically for maintenance/monitoring)
	if scriptValue, exists := challenge.Scripts["restart"]; exists && scriptValue.GetCommand() != "" {
		log.InfoH3("Running restart script for %s", challenge.Name)
		if err := w.runScriptWithIntervalSupport(challenge, "restart"); err != nil {
			log.Error("⚠️  Restart script failed for %s: %v", challenge.Name, err)
			// Don't return error for restart script failure - it's not critical for deployment
		}
	}

	log.InfoH2("✅ Successfully deployed new challenge: %s", challenge.Name)
}

// syncNewChallenge syncs a newly detected challenge to the GZCTF platform (deprecated - use syncAndDeployNewChallenge)
func (w *Watcher) syncNewChallenge(challenge ChallengeYaml) {
	// Get a mutex for this challenge to prevent race conditions
	mutex := w.getChallengeUpdateMutex(challenge.Name)
	mutex.Lock()
	defer mutex.Unlock()

	log.InfoH2("🚀 Starting sync for new challenge: %s", challenge.Name)

	// Get config and API setup
	config, err := GetConfig(w.gz.api)
	if err != nil {
		log.Error("❌ Failed to get config for new challenge sync %s: %v", challenge.Name, err)
		return
	}
	config.Event.CS = w.gz.api

	// Get existing challenges from API
	existingChallenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("❌ Failed to get API challenges for new challenge sync %s: %v", challenge.Name, err)
		return
	}

	// Sync the challenge (this will create it if it doesn't exist)
	if err := syncChallenge(config, challenge, existingChallenges, w.gz.api); err != nil {
		log.Error("❌ Failed to sync new challenge %s: %v", challenge.Name, err)
		return
	}

	log.InfoH2("✅ Successfully synced new challenge: %s", challenge.Name)
}

// isInChallengeCategory checks if a file path is within a challenge category directory
func (w *Watcher) isInChallengeCategory(filePath string) bool {
	// Get the challenge categories
	challengeCategories := []string{
		"Misc", "Crypto", "Pwn",
		"Web", "Reverse", "Blockchain",
		"Forensics", "Hardware", "Mobile", "PPC",
		"OSINT", "Game Hacking", "AI", "Pentest",
	}

	// Normalize the file path
	normalizedPath := filepath.Clean(filePath)
	pathParts := strings.Split(normalizedPath, string(filepath.Separator))

	// Check if any part of the path matches a challenge category
	for _, part := range pathParts {
		for _, category := range challengeCategories {
			if strings.EqualFold(part, category) {
				return true
			}
		}
	}

	return false
}

// GetDaemonStatus returns the status of the daemon watcher
func (w *Watcher) GetDaemonStatus(pidFile string) map[string]interface{} {
	if pidFile == "" {
		pidFile = DefaultWatcherConfig.PidFile
	}

	status := map[string]interface{}{
		"daemon":   false,
		"pid_file": pidFile,
	}

	pid, err := readPIDFromFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			status["status"] = "stopped"
			status["message"] = "PID file not found"
		} else {
			status["status"] = "error"
			status["message"] = err.Error()
		}
		return status
	}

	status["pid"] = pid

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		status["status"] = "error"
		status["message"] = fmt.Sprintf("Failed to find process: %v", err)
		return status
	}

	// Send signal 0 to check if process exists
	if err := process.Signal(syscall.Signal(0)); err != nil {
		status["daemon"] = false
		status["status"] = "dead"
		status["message"] = "Process not running (stale PID file)"
		// Clean up stale PID file
		if removeErr := os.Remove(pidFile); removeErr != nil && !os.IsNotExist(removeErr) {
			status["message"] = fmt.Sprintf("Process not running, failed to clean stale PID file: %v", removeErr)
		} else {
			status["message"] = "Process not running (cleaned up stale PID file)"
		}
		return status
	}

	status["daemon"] = true
	status["status"] = "running"
	status["message"] = "Daemon is running"
	return status
}

// StopDaemon stops the daemon watcher
func (w *Watcher) StopDaemon(pidFile string) error {
	if pidFile == "" {
		pidFile = DefaultWatcherConfig.PidFile
	}

	// Read PID from file
	pid, err := readPIDFromFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon is not running (PID file not found)")
		}
		return err
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM first
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
	}

	// Wait a bit for graceful shutdown
	time.Sleep(2 * time.Second)

	// Check if process is still running
	if err := process.Signal(syscall.Signal(0)); err == nil {
		// Process is still running, send SIGKILL
		log.Info("Process still running, sending SIGKILL...")
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", pid, err)
		}
	}

	// Clean up PID file
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	log.Info("✅ GZCTF Watcher daemon stopped successfully")
	return nil
}

// ShowStatus displays the watcher status
func (w *Watcher) ShowStatus(pidFile, logFile string, jsonOutput bool) error {
	if pidFile == "" {
		pidFile = DefaultWatcherConfig.PidFile
	}
	if logFile == "" {
		logFile = DefaultWatcherConfig.LogFile
	}

	daemonStatus := w.GetDaemonStatus(pidFile)
	isDaemon := daemonStatus["daemon"].(bool)
	daemonState := daemonStatus["status"].(string)

	log.Info("🔍 GZCTF Watcher Status")
	log.Info("==========================================")

	if isDaemon && daemonState == "running" {
		log.Info("🟢 Status: RUNNING (Daemon Mode)")
		if pid, ok := daemonStatus["pid"]; ok {
			log.Info("📄 Process ID: %v", pid)
		}
		log.Info("📄 PID File: %s", pidFile)
		log.Info("📝 Log File: %s", logFile)

		// For running daemon, try to get challenge info from status
		log.Info("")
		log.Info("📁 Configuration:")
		log.Info("   - Daemon Mode: Enabled")
		log.Info("   - PID File: %s", pidFile)
		log.Info("   - Log File: %s", logFile)
		log.Info("   - Git Pull: %v", DefaultWatcherConfig.GitPullEnabled)
		if DefaultWatcherConfig.GitPullEnabled {
			log.Info("   - Git Pull Interval: %v", DefaultWatcherConfig.GitPullInterval)
			log.Info("   - Git Repository: %s", DefaultWatcherConfig.GitRepository)
		}

		// Show recent log entries if available
		w.showRecentLogs(logFile)

	} else if daemonState == "dead" {
		log.Info("🟡 Status: STOPPED (Stale PID file found)")
		log.Info("💬 A previous daemon process was running but is no longer active")
		log.Info("📄 Stale PID File: %s", pidFile)
		log.Info("🔧 Suggestion: Run 'gzcli --watch' to start a new daemon")

	} else if daemonState == "stopped" {
		log.Info("⚫ Status: NOT RUNNING")
		log.Info("💬 No daemon is currently running")
		log.Info("📄 PID File: %s (not found)", pidFile)
		log.Info("🔧 Suggestion: Run 'gzcli --watch' to start the daemon")

	} else {
		log.Info("🔴 Status: ERROR")
		if msg, ok := daemonStatus["message"]; ok {
			log.Info("💬 %s", msg)
		}
		log.Info("📄 PID File: %s", pidFile)
	}

	log.Info("")
	log.Info("🛠️  Available Commands:")
	log.Info("   - Start daemon: gzcli --watch")
	log.Info("   - Stop daemon:  gzcli --watch-stop")
	log.Info("   - Run foreground: gzcli --watch --watch-foreground")

	// Output JSON format if requested
	if jsonOutput {
		return w.outputStatusJSON(daemonStatus, pidFile, logFile, isDaemon, daemonState)
	}

	return nil
}

// outputStatusJSON outputs status in JSON format
func (w *Watcher) outputStatusJSON(daemonStatus map[string]interface{}, pidFile, logFile string, isDaemon bool, daemonState string) error {
	// Create a cleaner status object for JSON
	jsonStatus := map[string]interface{}{
		"daemon_running": isDaemon && daemonState == "running",
		"status":         daemonState,
		"pid_file":       pidFile,
		"log_file":       logFile,
	}

	if isDaemon && daemonState == "running" {
		if pid, ok := daemonStatus["pid"]; ok {
			jsonStatus["pid"] = pid
		}
	}

	if msg, ok := daemonStatus["message"]; ok {
		jsonStatus["message"] = msg
	}

	log.Info("")
	jsonData, err := json.MarshalIndent(jsonStatus, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status to JSON: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}

// showRecentLogs displays recent log entries if the log file exists
func (w *Watcher) showRecentLogs(logFile string) {
	if _, err := os.Stat(logFile); err != nil {
		return // Log file doesn't exist
	}

	log.Info("")
	log.Info("📋 Recent Activity (last 5 lines from log):")

	// Use tail command to get last few lines
	cmd := exec.Command("tail", "-n", "5", logFile)
	output, err := cmd.Output()
	if err != nil {
		log.Info("   (Unable to read log file)")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			log.Info("   %s", strings.TrimSpace(line))
		}
	}
}

// writePIDFile writes the PID to the specified file
func (w *Watcher) writePIDFile(pidFile string, pid int) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(pidFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	// Write PID to file
	pidStr := fmt.Sprintf("%d\n", pid)
	if err := os.WriteFile(pidFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	log.Info("✅ PID file written successfully: %s", pidFile)
	return nil
}

// readPIDFromFile reads a PID integer from the given pid file.
// Returns os.ErrNotExist if the file does not exist, or a formatted error for invalid/empty PID content.
func readPIDFromFile(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, fmt.Errorf("PID file is empty")
	}
	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}
	return pid, nil
}

// gitPullLoop periodically pulls from git repository
func (w *Watcher) gitPullLoop(config WatcherConfig) {
	// Additional safeguard against zero duration
	interval := config.GitPullInterval
	if interval <= 0 {
		interval = DefaultWatcherConfig.GitPullInterval
		log.Error("GitPullInterval was zero or negative in loop, using default: %v", interval)
	}

	// Initial pull on startup
	log.Info("🔄 Performing initial git pull...")
	if err := w.performGitPull(config.GitRepository); err != nil {
		log.Error("Initial git pull failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Info("Git pull loop stopped")
			return
		case <-ticker.C:
			if err := w.performGitPull(config.GitRepository); err != nil {
				log.Error("Git pull failed: %v", err)
			}
		}
	}
}

// performGitPull performs a git pull operation on the specified repository
func (w *Watcher) performGitPull(repoPath string) error {
	log.InfoH3("🔄 Pulling latest changes from git repository: %s", repoPath)

	// Detect repository root (handle invocation from subdirectories)
	root := repoPath
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		if detected, findErr := findGitRepoRoot(repoPath); findErr == nil {
			log.Info("Detected git repository root at: %s", detected)
			root = detected
		} else {
			return fmt.Errorf("failed to locate git repository at %s: %w", repoPath, findErr)
		}
	}

	// Execute system git pull (inherits env; uses current credentials/SSH config)
	cmd := exec.Command("git", "-C", root, "pull")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("git pull failed: %v", err)
		if len(output) > 0 {
			log.Error("git output: %s", strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("git pull failed: %w", err)
	}

	// Log concise success and any non-empty output
	out := strings.TrimSpace(string(output))
	if out == "Already up to date." || strings.Contains(out, "Already up to date") {
		log.InfoH3("📄 Repository is already up-to-date")
	} else if out != "" {
		log.InfoH3("✅ Git pull output:\n%s", out)
	} else {
		log.InfoH3("✅ Git pull completed successfully")
	}

	// After successful pull, check for new challenges
	log.InfoH3("🔍 Checking for new challenges after git pull...")
	w.checkForNewChallenges()

	return nil
}

// findGitRepoRoot walks up from startPath to find a directory containing a .git folder
func findGitRepoRoot(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %s: %w", startPath, err)
	}

	current := absPath
	for {
		gitDir := filepath.Join(current, ".git")
		if info, statErr := os.Stat(gitDir); statErr == nil && info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no .git directory found from %s up to filesystem root", absPath)
		}
		current = parent
	}
}

// FollowLogs follows a log file and displays new content in real-time
func (w *Watcher) FollowLogs(logFile string) error {
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Tail the log file with re-open and follow options to handle rotations
	t, err := tail.TailFile(logFile, tail.Config{
		ReOpen:    true,
		Follow:    true,
		MustExist: false,
		Poll:      true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
	})
	if err != nil {
		return fmt.Errorf("failed to tail log file: %w", err)
	}
	defer t.Cleanup()

	// Print a header and the last few lines for context
	w.showRecentLogs(logFile)
	fmt.Println()

	for {
		select {
		case <-sigChan:
			fmt.Println("\n📋 Log following stopped.")
			return nil
		case line, ok := <-t.Lines:
			if !ok {
				return fmt.Errorf("log tail channel closed")
			}
			if line == nil {
				continue
			}
			text := line.Text
			if strings.TrimSpace(text) == "" {
				continue
			}
			if strings.Contains(text, "[x]") || strings.Contains(text, "INFO") || strings.Contains(text, "ERROR") {
				fmt.Println(text)
			} else {
				fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), text)
			}
		}
	}
}
