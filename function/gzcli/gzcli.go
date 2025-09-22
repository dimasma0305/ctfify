package gzcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
)

type Config struct {
	Url         string      `yaml:"url"`
	Creds       gzapi.Creds `yaml:"creds"`
	Event       gzapi.Game  `yaml:"event"`
	appsettings *AppSettings
}

type Container struct {
	FlagTemplate         string `yaml:"flagTemplate"`
	ContainerImage       string `yaml:"containerImage"`
	MemoryLimit          int    `yaml:"memoryLimit"`
	CpuCount             int    `yaml:"cpuCount"`
	StorageLimit         int    `yaml:"storageLimit"`
	ContainerExposePort  int    `yaml:"containerExposePort"`
	EnableTrafficCapture bool   `yaml:"enableTrafficCapture"`
}

// ScriptConfig represents a script configuration that can be either a simple string
// or a complex object with interval and execute parameters
type ScriptConfig struct {
	Execute  string        `yaml:"execute,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

// ScriptValue holds either a simple command string or a complex ScriptConfig
type ScriptValue struct {
	Simple  string
	Complex *ScriptConfig
}

// UnmarshalYAML implements custom YAML unmarshaling for ScriptValue
func (sv *ScriptValue) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// First try to unmarshal as a simple string
	var simpleScript string
	if err := unmarshal(&simpleScript); err == nil {
		sv.Simple = simpleScript
		sv.Complex = nil
		return nil
	}

	// If that fails, try to unmarshal as a complex object
	var complexScript ScriptConfig
	if err := unmarshal(&complexScript); err == nil {
		sv.Simple = ""
		sv.Complex = &complexScript
		return nil
	} else {
		return fmt.Errorf("script value must be either a string or an object with 'execute' and 'interval' fields")
	}
}

// IsSimple returns true if this is a simple string command
func (sv *ScriptValue) IsSimple() bool {
	return sv.Simple != ""
}

// GetCommand returns the command to execute
func (sv *ScriptValue) GetCommand() string {
	if sv.IsSimple() {
		return sv.Simple
	}
	if sv.Complex != nil {
		return sv.Complex.Execute
	}
	return ""
}

// GetInterval returns the execution interval for complex scripts
func (sv *ScriptValue) GetInterval() time.Duration {
	if sv.Complex != nil {
		return sv.Complex.Interval
	}
	return 0
}

// HasInterval returns true if this script has an interval configured
func (sv *ScriptValue) HasInterval() bool {
	return sv.Complex != nil && sv.Complex.Interval > 0
}

type ChallengeYaml struct {
	Name        string                 `yaml:"name"`
	Author      string                 `yaml:"author"`
	Description string                 `yaml:"description"`
	Flags       []string               `yaml:"flags"`
	Value       int                    `yaml:"value"`
	Provide     *string                `yaml:"provide,omitempty"`
	Visible     *bool                  `yaml:"visible"`
	Type        string                 `yaml:"type"`
	Hints       []string               `yaml:"hints"`
	Container   Container              `yaml:"container"`
	Scripts     map[string]ScriptValue `yaml:"scripts"`
	Dashboard   *Dashboard             `yaml:"dashboard,omitempty"`
	Category    string                 `yaml:"-"`
	Cwd         string                 `yaml:"-"`
}

type Standing struct {
	Pos   int    `json:"pos"`
	Team  string `json:"team"`
	Score int    `json:"score"`
}

type CTFTimeFeed struct {
	Tasks     []string   `json:"tasks"`
	Standings []Standing `json:"standings"`
}

type GZ struct {
	api        *gzapi.GZAPI
	UpdateGame bool
	watcher    *Watcher
}

// Cache frequently used paths and configurations
var (
	workDirOnce   sync.Once
	cachedWorkDir string
)

const (
	maxParallelScripts = 10
	gzctfDir           = ".gzctf"
)

// getWorkDir returns the cached working directory
func getWorkDir() string {
	workDirOnce.Do(func() {
		cachedWorkDir, _ = os.Getwd()
	})
	return cachedWorkDir
}

// Optimized database query execution with prepared command
var dbQueryCmd = exec.Command(
	"docker", "compose", "exec", "-T", "db", "psql",
	"--user", "postgres", "-d", "gzctf", "-c",
)

func runDBQuery(query string) error {
	cmd := *dbQueryCmd // Copy base command
	cmd.Args = append(cmd.Args, query)
	cmd.Dir = filepath.Join(getWorkDir(), gzctfDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Error("Database query failed: %v", err)
		return err
	}
	return nil
}

// Concurrent-safe initialization with memoization
var initOnce sync.Once
var initGZ *GZ
var initErr error

func Init() (*GZ, error) {
	initOnce.Do(func() {
		config, err := GetConfig(&gzapi.GZAPI{})
		if err != nil {
			initErr = fmt.Errorf("config error: %w", err)
			return
		}

		api, err := gzapi.Init(config.Url, &config.Creds)
		if err == nil {
			initGZ = &GZ{api: api}
			return
		}

		// Fallback to registration
		api, err = gzapi.Register(config.Url, &gzapi.RegisterForm{
			Email:    "admin@localhost",
			Username: config.Creds.Username,
			Password: config.Creds.Password,
		})
		if err != nil {
			initErr = fmt.Errorf("registration failed: %w", err)
			return
		}

		if err := runDBQuery(fmt.Sprintf(
			`UPDATE "AspNetUsers" SET "Role"=3 WHERE "UserName"='%s';`,
			config.Creds.Username,
		)); err != nil {
			initErr = err
			return
		}

		initGZ = &GZ{api: api}
	})
	return initGZ, initErr
}

func (gz *GZ) GenerateStructure() error {
	appsetings, err := getAppSettings()
	if err != nil {
		return err
	}
	challenges, err := GetChallengesYaml(&Config{appsettings: appsetings})
	if err != nil {
		return err
	}

	// Call genStructure with the provided challenges
	if err := genStructure(challenges); err != nil {
		log.Error("Failed to generate structure: %v", err)
		return err
	}
	return nil
}

// Bulk game deletion with parallel execution
func (gz *GZ) RemoveAllEvent() error {
	games, err := gz.api.GetGames()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(games))
	sem := make(chan struct{}, 5) // Limit concurrent deletions

	for _, game := range games {
		wg.Add(1)
		sem <- struct{}{}
		go func(g gzapi.Game) {
			defer func() {
				<-sem
				wg.Done()
			}()
			if err := g.Delete(); err != nil {
				errChan <- err
			}
		}(*game)
	}

	wg.Wait()
	close(errChan)

	return <-errChan // Return first error if any
}

// Preallocated scoreboard generation
func (gz *GZ) Scoreboard2CTFTimeFeed() (*CTFTimeFeed, error) {
	config, err := GetConfig(gz.api)
	if err != nil {
		return nil, err
	}

	scoreboard, err := config.Event.GetScoreboard()
	if err != nil {
		return nil, fmt.Errorf("scoreboard error: %w", err)
	}

	feed := &CTFTimeFeed{
		Standings: make([]Standing, 0, len(scoreboard.Items)),
		Tasks:     make([]string, 0, len(scoreboard.Challenges)*5),
	}

	for _, item := range scoreboard.Items {
		feed.Standings = append(feed.Standings, Standing{
			Pos:   item.Rank,
			Team:  item.Name,
			Score: item.Score,
		})
	}

	for category, items := range scoreboard.Challenges {
		for _, item := range items {
			feed.Tasks = append(feed.Tasks, fmt.Sprintf("%s - %s", category, item.Title))
		}
	}
	return feed, nil
}

// Optimized script runner with worker pool
func RunScripts(script string) error {
	appsetings, err := getAppSettings()
	if err != nil {
		return err
	}
	challengesConf, err := GetChallengesYaml(&Config{appsettings: appsetings})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workChan := make(chan ChallengeYaml, len(challengesConf))
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	// Create worker pool
	for i := 0; i < maxParallelScripts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for challengeConf := range workChan {
				select {
				case <-ctx.Done():
					return
				default:
					if err := runScript(challengeConf, script); err != nil {
						select {
						case errChan <- fmt.Errorf("script error in %s: %w", challengeConf.Name, err):
							cancel()
						default:
						}
					}
				}
			}
		}()
	}

	// Distribute work
	for _, conf := range challengesConf {
		if scriptValue, ok := conf.Scripts[script]; ok && scriptValue.GetCommand() != "" {
			workChan <- conf
		}
	}
	close(workChan)
	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

func (gz *GZ) Sync() error {
	log.Info("Starting sync process...")

	config, err := GetConfig(gz.api)
	if err != nil {
		log.Error("Failed to get config: %v", err)
		return fmt.Errorf("config error: %w", err)
	}
	log.Info("Config loaded successfully")

	// Get fresh challenges config
	log.Info("Loading challenges configuration...")
	challengesConf, err := GetChallengesYaml(config)
	if err != nil {
		log.Error("Failed to get challenges YAML: %v", err)
		return fmt.Errorf("challenges config error: %w", err)
	}
	log.Info("Loaded %d challenges from configuration", len(challengesConf))

	// Get fresh games list
	log.Info("Fetching games from API...")
	games, err := gz.api.GetGames()
	if err != nil {
		log.Error("Failed to get games: %v", err)
		return fmt.Errorf("games fetch error: %w", err)
	}
	log.Info("Found %d games", len(games))

	currentGame := findCurrentGame(games, config.Event.Title, gz.api)
	if currentGame == nil {
		log.Info("Current game not found, clearing cache and retrying...")
		DeleteCache("config")
		return gz.Sync()
	}
	log.Info("Found current game: %s (ID: %d)", currentGame.Title, currentGame.Id)

	if gz.UpdateGame {
		log.Info("Updating game configuration...")
		if err := updateGameIfNeeded(config, currentGame, gz.api); err != nil {
			log.Error("Failed to update game: %v", err)
			return fmt.Errorf("game update error: %w", err)
		}
		log.Info("Game updated successfully")
	}

	log.Info("Validating challenges...")
	if err := validateChallenges(challengesConf); err != nil {
		log.Error("Challenge validation failed: %v", err)
		return fmt.Errorf("validation error: %w", err)
	}
	log.Info("All challenges validated successfully")

	// Get fresh challenges list
	log.Info("Fetching existing challenges from API...")
	config.Event.CS = gz.api
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		log.Error("Failed to get challenges from API: %v", err)
		return fmt.Errorf("API challenges fetch error: %w", err)
	}
	log.Info("Found %d existing challenges in API", len(challenges))

	// Process challenges
	log.Info("Starting challenge synchronization...")
	var wg sync.WaitGroup
	errChan := make(chan error, len(challengesConf))
	successCount := 0
	failureCount := 0

	// Create per-challenge mutexes to prevent race conditions
	challengeMutexes := make(map[string]*sync.Mutex)
	var mutexesMu sync.RWMutex

	for _, conf := range challengesConf {
		wg.Add(1)
		go func(c ChallengeYaml) {
			defer wg.Done()

			// Get or create mutex for this challenge to prevent duplicates
			mutexesMu.Lock()
			if challengeMutexes[c.Name] == nil {
				challengeMutexes[c.Name] = &sync.Mutex{}
			}
			mutex := challengeMutexes[c.Name]
			mutexesMu.Unlock()

			// Synchronize access per challenge to prevent race conditions
			mutex.Lock()
			defer mutex.Unlock()

			log.Info("Processing challenge: %s", c.Name)
			if err := syncChallenge(config, c, challenges, gz.api); err != nil {
				log.Error("Failed to sync challenge %s: %v", c.Name, err)
				errChan <- fmt.Errorf("challenge sync failed for %s: %w", c.Name, err)
				failureCount++
			} else {
				log.Info("Successfully synced challenge: %s", c.Name)
				successCount++
			}
		}(conf)
	}

	wg.Wait()
	close(errChan)

	log.Info("Sync completed. Success: %d, Failures: %d", successCount, failureCount)

	// Return first error if any
	select {
	case err := <-errChan:
		return err
	default:
		log.Info("All challenges synced successfully!")
		return nil
	}
}

// MustInit initializes GZ or fatally logs error
func MustInit() *GZ {
	gz, err := Init()
	if err != nil {
		log.Fatal("Initialization failed: ", err)
	}
	return gz
}

// MustSync synchronizes data or fatally logs error
func (gz *GZ) MustSync() {
	if err := gz.Sync(); err != nil {
		log.Fatal("Sync failed: ", err)
	}
}

func (gz *GZ) MustScoreboard2CTFTimeFeed() *CTFTimeFeed {
	feed, err := gz.Scoreboard2CTFTimeFeed()
	if err != nil {
		log.Fatal("Scoreboard generation failed: ", err)
	}
	return feed
}

// MustRunScripts executes scripts or fatally logs error
func MustRunScripts(script string) {
	if err := RunScripts(script); err != nil {
		log.Fatal("Script execution failed: ", err)
	}
}

// MustCreateTeams creates teams or fatally logs error
func (gz *GZ) MustCreateTeams(url string, sendEmail bool) {
	if err := gz.CreateTeams(url, sendEmail); err != nil {
		log.Fatal("Team creation failed: ", err)
	}
}

// MustDeleteAllUser removes all users or fatally logs error
func (gz *GZ) MustDeleteAllUser() {
	if err := gz.DeleteAllUser(); err != nil {
		log.Fatal("User deletion failed: ", err)
	}
}

// StartWatcher starts the file watcher service
func (gz *GZ) StartWatcher(config WatcherConfig) error {
	if gz.watcher != nil && gz.watcher.IsWatching() {
		return fmt.Errorf("watcher is already running")
	}

	watcher, err := NewWatcher(gz)
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	if err := watcher.Start(config); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	gz.watcher = watcher
	return nil
}

// StopWatcher stops the file watcher service
func (gz *GZ) StopWatcher() error {
	if gz.watcher == nil {
		return fmt.Errorf("no watcher is running")
	}

	if err := gz.watcher.Stop(); err != nil {
		return fmt.Errorf("failed to stop watcher: %w", err)
	}

	gz.watcher = nil
	return nil
}

// IsWatcherRunning returns true if the watcher is currently running
func (gz *GZ) IsWatcherRunning() bool {
	return gz.watcher != nil && gz.watcher.IsWatching()
}

// GetWatcherStatus returns the status of the watcher service
func (gz *GZ) GetWatcherStatus() map[string]interface{} {
	status := map[string]interface{}{
		"running": gz.IsWatcherRunning(),
	}

	if gz.watcher != nil {
		status["watched_challenges"] = gz.watcher.GetWatchedChallenges()
	} else {
		status["watched_challenges"] = []string{}
	}

	return status
}

// MustStartWatcher starts the watcher or fatally logs error
func (gz *GZ) MustStartWatcher(config WatcherConfig) {
	if err := gz.StartWatcher(config); err != nil {
		log.Fatal("Failed to start watcher: ", err)
	}
}

// MustStopWatcher stops the watcher or fatally logs error
func (gz *GZ) MustStopWatcher() {
	if err := gz.StopWatcher(); err != nil {
		log.Fatal("Failed to stop watcher: ", err)
	}
}
