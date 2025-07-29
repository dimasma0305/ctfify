package gzcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
)

const (
	GZCTF_DIR        = ".gzctf"
	CONFIG_FILE      = "conf.yaml"
	APPSETTINGS_FILE = "appsettings.json"
)

var (
	CHALLENGE_CATEGORY = []string{
		"Misc", "Crypto", "Pwn",
		"Web", "Reverse", "Blockchain",
		"Forensics", "Hardware", "Mobile", "PPC",
		"OSINT", "Game Hacking", "AI", "Pentest",
	}
	challengeFileRegex = regexp.MustCompile(`challenge\.(yaml|yml)$`)
	slugRegex          = regexp.MustCompile(`[^a-z0-9_]+`)
)

// Cache for parsed URL host
var hostCache struct {
	host string
	once sync.Once
}

func GetConfig(api *gzapi.GZAPI) (*Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var config Config
	confPath := filepath.Join(dir, GZCTF_DIR, CONFIG_FILE)
	if err := ParseYamlFromFile(confPath, &config); err != nil {
		return nil, err
	}

	// First, try to get cached config
	var configCache Config
	cacheErr := GetCache("config", &configCache)

	// If we have cached game info, use it as the starting point
	if cacheErr == nil && configCache.Event.Id != 0 {
		config.Event.Id = configCache.Event.Id
		config.Event.PublicKey = configCache.Event.PublicKey
		log.Info("Using cached game ID: %d", config.Event.Id)
	}

	// Only interact with API if provided and we need to validate/create game
	if api != nil && api.Client != nil {
		// If we have a cached game ID, try to validate it exists
		if config.Event.Id != 0 {
			log.Info("Validating cached game ID %d exists on server...", config.Event.Id)
			games, err := api.GetGames()
			if err != nil {
				log.Error("Failed to get games for validation: %v", err)
				return nil, fmt.Errorf("API games fetch error: %w", err)
			}

			// Check if the cached game ID still exists
			gameExists := false
			for _, game := range games {
				if game.Id == config.Event.Id {
					gameExists = true
					// Update with current server data but keep the same ID
					config.Event.PublicKey = game.PublicKey
					log.Info("Cached game ID %d validated successfully", config.Event.Id)
					break
				}
			}

			// If cached game doesn't exist, clear cache and try to find by title
			if !gameExists {
				log.Info("Cached game ID %d not found on server, searching by title...", config.Event.Id)
				DeleteCache("config")
				config.Event.Id = 0
				config.Event.PublicKey = ""
			}
		}

		// If we don't have a valid game ID, try to find by title or create new
		if config.Event.Id == 0 {
			game, err := api.GetGameByTitle(config.Event.Title)
			if err != nil {
				log.Info("Game '%s' not found by title, creating new game...", config.Event.Title)
				game, err = createNewGame(&config, api)
				if err != nil {
					return nil, fmt.Errorf("failed to create new game: %w", err)
				}
			} else {
				log.Info("Found existing game by title: %s (ID: %d)", game.Title, game.Id)
				config.Event.Id = game.Id
				config.Event.PublicKey = game.PublicKey
				// Update cache with found game
				if err := setCache("config", &config); err != nil {
					log.Error("Failed to update cache with found game: %v", err)
				}
			}
		}
	}

	config.appsettings, err = getAppSettings()
	if err != nil {
		return nil, fmt.Errorf("errror parsing appsettings.json: %s", err)
	}
	return &config, nil
}

func getAppSettings() (*AppSettings, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	appSettingsPath := filepath.Join(dir, GZCTF_DIR, APPSETTINGS_FILE)
	content, err := os.ReadFile(appSettingsPath)
	if err != nil {
		return nil, fmt.Errorf("reading appsettings file error: %w", err)
	}

	var settings AppSettings
	if err := json.Unmarshal(content, &settings); err != nil {
		return nil, fmt.Errorf("unmarshalling appsettings error: %w", err)
	}

	return &settings, nil
}

func generateSlug(challengeConf ChallengeYaml) string {
	var b strings.Builder
	b.Grow(len(challengeConf.Category) + len(challengeConf.Name) + 1)

	b.WriteString(strings.ToLower(challengeConf.Category))
	b.WriteByte('_')
	b.WriteString(strings.ToLower(challengeConf.Name))

	slug := strings.ReplaceAll(b.String(), " ", "_")
	return slugRegex.ReplaceAllString(slug, "")
}

func GetChallengesYaml(config *Config) ([]ChallengeYaml, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Pre-parse URL once
	hostCache.once.Do(func() {
		hostCache.host = config.appsettings.ContainerProvider.PublicEntry
	})

	var wg sync.WaitGroup
	challengeChan := make(chan ChallengeYaml)
	errChan := make(chan error, 1)
	resultChan := make(chan []ChallengeYaml)

	// Start result collector
	go func() {
		var challenges []ChallengeYaml
		for c := range challengeChan {
			challenges = append(challenges, c)
		}
		resultChan <- challenges
	}()

	// Process categories in parallel
	for _, category := range CHALLENGE_CATEGORY {
		wg.Add(1)
		go func(category string) {
			defer wg.Done()
			categoryPath := filepath.Join(dir, category)

			if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
				return
			}

			err := filepath.Walk(categoryPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !challengeFileRegex.MatchString(info.Name()) {
					return err
				}

				content, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading file error: %w", err)
				}

				var challenge ChallengeYaml
				if err := ParseYamlFromBytes(content, &challenge); err != nil {
					return err
				}

				challenge.Category = category
				challenge.Cwd = filepath.Dir(path)

				if category == "Game Hacking" {
					challenge.Category = "Reverse"
					challenge.Name = "[Game Hacking] " + challenge.Name
				}

				t, err := template.New("chall").Parse(string(content))
				if err != nil {
					log.ErrorH2("template error: %v", err)
					return nil
				}

				var buf bytes.Buffer
				err = t.Execute(&buf, map[string]string{
					"host": hostCache.host,
					"slug": generateSlug(challenge),
				})
				if err != nil {
					return fmt.Errorf("template execution error: %w", err)
				}

				if err := ParseYamlFromBytes(buf.Bytes(), &challenge); err != nil {
					return fmt.Errorf("yaml parse error: %w", err)
				}

				select {
				case challengeChan <- challenge:
				case <-errChan:
				}
				return nil
			})

			if err != nil {
				select {
				case errChan <- fmt.Errorf("category %s: %w", category, err):
				default:
				}
			}
		}(category)
	}

	go func() {
		wg.Wait()
		close(challengeChan)
	}()

	select {
	case err := <-errChan:
		close(errChan)
		return nil, err
	case challenges := <-resultChan:
		return challenges, nil
	}
}
