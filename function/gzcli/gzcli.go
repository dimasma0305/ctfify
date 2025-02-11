package gzcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
)

type Config struct {
	Url   string      `yaml:"url"`
	Creds gzapi.Creds `yaml:"creds"`
	Event gzapi.Game  `yaml:"event"`
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

type ChallengeYaml struct {
	Name        string            `yaml:"name"`
	Author      string            `yaml:"author"`
	Description string            `yaml:"description"`
	Flags       []string          `yaml:"flags"`
	Value       int               `yaml:"value"`
	Provide     *string           `yaml:"provide,omitempty"`
	Visible     *bool             `yaml:"visible"`
	Type        string            `yaml:"type"`
	Hints       []string          `yaml:"hints"`
	Container   Container         `yaml:"container"`
	Scripts     map[string]string `yaml:"scripts"`
	Category    string            `yaml:"-"`
	Cwd         string            `yaml:"-"`
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
	challengesConf, err := GetChallengesYaml(&Config{})
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
		if _, ok := conf.Scripts[script]; ok {
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
	config, err := GetConfig(gz.api)
	if err != nil {
		return err
	}

	// Get fresh challenges config
	challengesConf, err := GetChallengesYaml(config)
	if err != nil {
		return err
	}

	// Get fresh games list
	games, err := gz.api.GetGames()
	if err != nil {
		return err
	}

	currentGame := findCurrentGame(games, config.Event.Title, gz.api)
	if currentGame == nil {
		DeleteCache("config")
		return gz.Sync()
	}

	if gz.UpdateGame {
		if err := updateGameIfNeeded(config, currentGame, gz.api); err != nil {
			return err
		}
	}

	if err := validateChallenges(challengesConf); err != nil {
		return err
	}

	// Get fresh challenges list
	config.Event.CS = gz.api
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return err
	}

	// Process challenges
	var wg sync.WaitGroup
	errChan := make(chan error, len(challengesConf))

	for _, conf := range challengesConf {
		wg.Add(1)
		go func(c ChallengeYaml) {
			defer wg.Done()
			if err := syncChallenge(config, c, challenges, gz.api); err != nil {
				errChan <- err
			}
		}(conf)
	}

	wg.Wait()
	close(errChan)

	// Return first error if any
	select {
	case err := <-errChan:
		return err
	default:
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
