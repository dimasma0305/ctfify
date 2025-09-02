package gzcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/sevlyar/go-daemon"
)

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
	daemonContext        *daemon.Context   // Daemon context for process management
	watchedChallengeDirs map[string]string // challengeName -> cwd
	challengeConfigs     map[string]ChallengeYaml // challengeName -> full config (for scripts)
	challengeConfigsMu   sync.RWMutex
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
	// Get challenges and add them to watch
	challenges, err := w.getChallenges()
	if err != nil {
		return fmt.Errorf("failed to get challenges: %w", err)
	}

	for _, challenge := range challenges {
		if err := w.addChallengeToWatch(challenge); err != nil {
			log.Error("Failed to watch challenge %s: %v", challenge.Name, err)
			continue
		}
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

	log.Info("📁 Watching %d challenges", len(challenges))
	log.Info("File watcher started successfully")

	return nil
}

// Stop stops the file watcher
func (w *Watcher) Stop() error {
	log.Info("Stopping file watcher...")
	w.cancel()

	// Wait for all goroutines to finish
	w.wg.Wait()

	if w.watcher != nil {
		err := w.watcher.Close()
		if err != nil {
			return fmt.Errorf("failed to close watcher: %w", err)
		}
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

	// Run stop script if available
	if _, exists := challenge.Scripts["stop"]; exists {
		log.InfoH3("Running stop script for %s", challenge.Name)
		if err := runScript(challenge, "stop"); err != nil {
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
	if _, exists := challenge.Scripts["stop"]; exists {
		log.InfoH3("Running stop script for %s", challenge.Name)
		if err := runScript(challenge, "stop"); err != nil {
			log.Error("Stop script failed for %s: %v", challenge.Name, err)
			// Continue anyway - maybe the service wasn't running
		}
	}

	// Update metadata and attachment
	if err := w.updateMetadataAndAttachment(challenge); err != nil {
		return fmt.Errorf("failed to update challenge metadata: %w", err)
	}

	// Run start script if exists
	if _, exists := challenge.Scripts["start"]; exists {
		log.InfoH3("Running start script for %s", challenge.Name)
		if err := runScript(challenge, "start"); err != nil {
			return fmt.Errorf("start script failed: %w", err)
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
			continue
		}
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
	if _, exists := challenge.Scripts["start"]; exists {
		log.InfoH3("Running start script for %s", challenge.Name)
		if err := runScript(challenge, "start"); err != nil {
			log.Error("❌ Start script failed for %s: %v", challenge.Name, err)
			return
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
