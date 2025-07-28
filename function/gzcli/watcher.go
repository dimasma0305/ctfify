package gzcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	gz                 *GZ
	watcher            *fsnotify.Watcher
	config             WatcherConfig
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	debounceTimers     map[string]*time.Timer
	debounceTimersMu   sync.RWMutex
	challengeMutexes   map[string]*sync.Mutex
	challengeMutexesMu sync.RWMutex
	pendingUpdates     map[string]string // challengeName -> latest file path
	pendingUpdatesMu   sync.RWMutex
	updatingChallenges map[string]bool // challengeName -> is updating
	updatingMu         sync.RWMutex
	watchedChallenges  map[string]bool // challengeName -> is being watched
	watchedMu          sync.RWMutex
}

type WatcherConfig struct {
	PollInterval              time.Duration
	DebounceTime              time.Duration
	IgnorePatterns            []string
	WatchPatterns             []string
	EnableGitPull             bool
	GitPullInterval           time.Duration
	NewChallengeCheckInterval time.Duration // New field for checking new challenges
}

var DefaultWatcherConfig = WatcherConfig{
	PollInterval:              5 * time.Second,
	DebounceTime:              2 * time.Second,
	IgnorePatterns:            []string{}, // No ignore patterns
	WatchPatterns:             []string{}, // Empty means watch all files
	EnableGitPull:             false,      // Disabled by default
	GitPullInterval:           5 * time.Second,
	NewChallengeCheckInterval: 10 * time.Second, // Check for new challenges every 10 seconds
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
		gz:                 gz,
		watcher:            watcher,
		ctx:                ctx,
		cancel:             cancel,
		debounceTimers:     make(map[string]*time.Timer),
		challengeMutexes:   make(map[string]*sync.Mutex),
		pendingUpdates:     make(map[string]string),
		updatingChallenges: make(map[string]bool),
		watchedChallenges:  make(map[string]bool),
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

	log.Info("Starting file watcher...")

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
		w.watchLoop(config)
	}()

	// Start git pull routine if enabled
	if config.EnableGitPull {
		log.Info("Git pull enabled, checking every %v", config.GitPullInterval)
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.gitPullLoop(config)
		}()
	}

	// Start new challenge checking routine
	log.Info("New challenge detection enabled, checking every %v", w.config.NewChallengeCheckInterval)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.newChallengeCheckLoop(w.config)
	}()

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

	// Mark as watched
	w.watchedMu.Lock()
	w.watchedChallenges[challenge.Name] = true
	w.watchedMu.Unlock()

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

				// Debounce the event using the watcher's debounce timers
				w.debounceTimersMu.Lock()
				if timer, exists := w.debounceTimers[event.Name]; exists {
					timer.Stop()
				}

				w.debounceTimers[event.Name] = time.AfterFunc(config.DebounceTime, func() {
					// Check if file still exists before processing
					if _, err := os.Stat(event.Name); err == nil {
						w.handleFileChange(event.Name)
					} else {
						log.DebugH3("Skipping file change for non-existent file: %s", event.Name)
					}
					w.debounceTimersMu.Lock()
					delete(w.debounceTimers, event.Name)
					w.debounceTimersMu.Unlock()
				})
				w.debounceTimersMu.Unlock()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error("Watcher error: %v", err)
		}
	}
}

// shouldProcessEvent determines if we should process a file system event
func (w *Watcher) shouldProcessEvent(event fsnotify.Event, config WatcherConfig) bool {
	// Only process Write and Create events to avoid loops
	if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 {
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

	log.DebugH3("Determining update type for: %s (relative: %s)", filePath, relPath)

	// Check if it's in solver directory - no update needed
	if strings.HasPrefix(relPath, "solver/") || strings.HasPrefix(relPath, "writeup/") {
		log.InfoH3("File is in solver/writeup directory, skipping update")
		return UpdateNone
	}

	// Check if it's challenge.yml - metadata update only
	if filepath.Base(relPath) == "challenge.yml" {
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

	// Default to metadata update for other files
	log.InfoH3("Other file changed, updating metadata and attachment")
	return UpdateMetadata
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

	// Check if this challenge is already being updated
	if w.isUpdating(challenge.Name) {
		log.InfoH3("Challenge %s is already being updated, setting as pending", challenge.Name)
		w.setPendingUpdate(challenge.Name, filePath)
		return
	}

	// Start the update process
	w.processUpdate(challenge.Name, filePath)
}

// processUpdate handles the actual update process and checks for pending updates
func (w *Watcher) processUpdate(challengeName, filePath string) {
	// Mark as updating
	w.setUpdating(challengeName, true)
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

// redeployChallenge redeploys a specific challenge
func (w *Watcher) redeployChallenge(challenge ChallengeYaml) error {
	log.InfoH2("Starting redeployment of challenge: %s", challenge.Name)

	// Run pre-deploy script if exists
	if _, exists := challenge.Scripts["predeploy"]; exists {
		log.InfoH3("Running predeploy script for %s", challenge.Name)
		if err := runScript(challenge, "predeploy"); err != nil {
			log.Error("Predeploy script failed for %s: %v", challenge.Name, err)
			// Continue anyway
		}
	}

	// Run build script if exists
	if _, exists := challenge.Scripts["build"]; exists {
		log.InfoH3("Running build script for %s", challenge.Name)
		if err := runScript(challenge, "build"); err != nil {
			return fmt.Errorf("build script failed: %w", err)
		}
	}

	// Sync the challenge with the API
	config, err := GetConfig(w.gz.api)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Ensure the API client is properly set
	config.Event.CS = w.gz.api

	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return fmt.Errorf("failed to get API challenges: %w", err)
	}

	if err := syncChallenge(config, challenge, challenges, w.gz.api); err != nil {
		return fmt.Errorf("failed to sync challenge: %w", err)
	}

	// Run deploy script if exists
	if _, exists := challenge.Scripts["deploy"]; exists {
		log.InfoH3("Running deploy script for %s", challenge.Name)
		if err := runScript(challenge, "deploy"); err != nil {
			return fmt.Errorf("deploy script failed: %w", err)
		}
	}

	// Run postdeploy script if exists
	if _, exists := challenge.Scripts["postdeploy"]; exists {
		log.InfoH3("Running postdeploy script for %s", challenge.Name)
		if err := runScript(challenge, "postdeploy"); err != nil {
			log.Error("Postdeploy script failed for %s: %v", challenge.Name, err)
			// Continue anyway
		}
	}

	return nil
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

// gitPullLoop runs git pull periodically
func (w *Watcher) gitPullLoop(config WatcherConfig) {
	ticker := time.NewTicker(config.GitPullInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Info("Git pull loop stopped")
			return
		case <-ticker.C:
			w.performGitPull()
		}
	}
}

// performGitPull executes git pull and handles the result
func (w *Watcher) performGitPull() {
	log.DebugH3("Checking for git updates...")

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Error("Failed to get current working directory: %v", err)
		return
	}

	// Check if this is a git repository
	gitDir := filepath.Join(cwd, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		log.DebugH3("Not a git repository, skipping git pull")
		return
	}

	// Execute git pull
	cmd := exec.CommandContext(w.ctx, "git", "pull")
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Error("Git pull failed: %v", err)
		log.DebugH3("Git pull output: %s", string(output))
		return
	}

	outputStr := strings.TrimSpace(string(output))

	// Check if there were any changes
	if strings.Contains(outputStr, "Already up to date") || strings.Contains(outputStr, "Already up-to-date") {
		log.DebugH3("Repository is up to date")
		return
	}

	// There were changes
	log.InfoH2("Git pull completed with changes:")
	log.InfoH3("%s", outputStr)

	// Parse the output to see what files changed
	lines := strings.Split(outputStr, "\n")
	changedFiles := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for file change patterns in git pull output
		if strings.Contains(line, "|") && (strings.Contains(line, "+") || strings.Contains(line, "-")) {
			// Extract filename (format: " filename.ext | 5 ++---")
			parts := strings.Split(line, "|")
			if len(parts) > 0 {
				filename := strings.TrimSpace(parts[0])
				if filename != "" {
					changedFiles = append(changedFiles, filename)
				}
			}
		}
	}

	// Process changed files
	if len(changedFiles) > 0 {
		log.InfoH3("Processing %d changed files from git pull", len(changedFiles))

		// Check if any of the changed files might be new challenge directories
		shouldCheckNewChallenges := false

		for _, file := range changedFiles {
			log.DebugH3("Git pull changed file: %s", file)

			// Check if this could be a new challenge directory
			// Look for challenge.yml files or directories that match challenge categories
			if strings.HasSuffix(file, "challenge.yml") || w.isInChallengeCategory(file) {
				shouldCheckNewChallenges = true
			}

			// Convert relative path to absolute path
			absPath := filepath.Join(cwd, file)
			if _, err := os.Stat(absPath); err == nil {
				// File exists, process the change
				go w.handleFileChange(absPath)
			}
		}

		// If we detected potential new challenges, check for them
		if shouldCheckNewChallenges {
			log.InfoH3("Potential new challenges detected, checking for new challenges...")

			// Get current challenges to check for new ones
			challenges, err := w.getChallenges()
			if err != nil {
				log.Error("Failed to get challenges for new challenge check after git pull: %v", err)
			} else {
				// This will both add to watcher and sync new challenges
				w.checkAndAddNewChallenges(challenges)
			}
		}
	} else {
		log.InfoH3("Git pull completed but no specific files detected, triggering general sync")
		// If we can't detect specific files, trigger a general sync for all challenges
		go w.handleGitPullGeneralSync()
	}
}

// handleGitPullGeneralSync triggers a sync for all challenges when specific files can't be detected
func (w *Watcher) handleGitPullGeneralSync() {
	log.InfoH2("Performing general sync after git pull")

	challenges, err := w.getChallenges()
	if err != nil {
		log.Error("Failed to get challenges for general sync: %v", err)
		return
	}

	// Check for new challenges first
	w.checkAndAddNewChallenges(challenges)

	// Process each challenge
	for _, challenge := range challenges {
		// Use challenge.yml as the trigger file for metadata update
		challengeYmlPath := filepath.Join(challenge.Cwd, "challenge.yml")
		if _, err := os.Stat(challengeYmlPath); err == nil {
			log.DebugH3("Triggering sync for challenge: %s", challenge.Name)
			w.handleFileChange(challengeYmlPath)
		}
	}
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

		// Sync the new challenge to GZCTF platform
		log.InfoH3("🔄 Syncing new challenge to GZCTF platform: %s", challenge.Name)
		go w.syncNewChallenge(challenge)
	}

	if len(newChallenges) == 0 {
		log.DebugH3("No new challenges found")
	} else {
		log.InfoH2("🎯 Found %d new challenge(s) - added to watcher and syncing to platform", len(newChallenges))
	}
}

// syncNewChallenge syncs a newly detected challenge to the GZCTF platform
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
