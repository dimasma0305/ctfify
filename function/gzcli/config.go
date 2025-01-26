package gzcli

import (
	"bytes"
	"fmt"
	"net/url"
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
	GZCTF_DIR   = ".gzctf"
	CONFIG_FILE = "conf.yaml"
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

	// Parallel check for cache and API
	var wg sync.WaitGroup
	var configCache Config
	var cacheErr, apiErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		cacheErr = GetCache("config", &configCache)
	}()

	if api != nil && api.Client != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			game, err := api.GetGameByTitle(config.Event.Title)
			if err != nil {
				game, apiErr = createNewGame(&config, api)
			}
			if game != nil {
				config.Event.Id = game.Id
				config.Event.PublicKey = game.PublicKey
			}
		}()
	}

	wg.Wait()

	if cacheErr == nil {
		config.Event.Id = configCache.Event.Id
		config.Event.PublicKey = configCache.Event.PublicKey
	}

	if apiErr != nil {
		return nil, apiErr
	}

	return &config, nil
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
		if config.Url != "" {
			if parsedURL, err := url.Parse(config.Url); err == nil {
				hostCache.host = parsedURL.Hostname()
			}
		}
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
