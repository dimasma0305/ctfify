package gzcli

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
)

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

	var configCache Config
	if err := GetCache("config", &configCache); err == nil {
		config.Event.Id = configCache.Event.Id
		config.Event.PublicKey = configCache.Event.PublicKey
	} else {
		if api != nil && api.Client != nil {
			game, err := api.GetGameByTitle(config.Event.Title)
			if err != nil {
				log.Error("error getting the game title: %s", err)
				game, err = createNewGame(&config, api)
				if err != nil {
					return nil, err
				}
			}
			config.Event.Id = game.Id
			config.Event.PublicKey = game.PublicKey
		}
	}
	return &config, nil
}
func generateSlug(challengeConf ChallengeYaml) string {
	slug := fmt.Sprintf("%s_%s", challengeConf.Category, challengeConf.Name)
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, " ", "_")

	re := regexp.MustCompile(`[^a-z0-9_]+`)
	slug = re.ReplaceAllString(slug, "")

	return slug
}

func GetChallengesYaml(config *Config) ([]ChallengeYaml, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	var challenges []ChallengeYaml
	ChallengeFilePattern := regexp.MustCompile(`challenge\.(yaml|yml)$`)
	for _, category := range CHALLENGE_CATEGORY {
		categoryPath := filepath.Join(dir, category)
		log.InfoH2("[%s]", category)
		if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(categoryPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if ChallengeFilePattern.MatchString(info.Name()) {
				var challenge ChallengeYaml
				if err := ParseYamlFromFile(path, &challenge); err != nil {
					return err
				}
				challenge.Category = category
				challenge.Cwd = filepath.Dir(path)
				log.InfoH3("'%s'", challenge.Cwd)
				if category == "Game Hacking" {
					challenge.Category = "Reverse"
					challenge.Name = "[Game Hacking] " + challenge.Name
				}
				yamlRaw, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading file challenge error: %s", err)
				}
				t, err := template.New("chall").Parse(string(yamlRaw))
				if err != nil {
					log.ErrorH2("error parsing template: %v", err)
					return nil
				}

				var parsedURL *url.URL
				if config.Url != "" {
					parsedURL, err = url.Parse(config.Url)
					if err != nil {
						log.Fatal(err)
					}
				}

				var buf bytes.Buffer
				host := ""
				if parsedURL != nil {
					host = parsedURL.Host
				}
				err = t.Execute(&buf, map[string]string{
					"host": host,
					"slug": generateSlug(challenge),
				})
				if err != nil {
					return fmt.Errorf("error executing template: %v", err)
				}

				if err := ParseYamlFromBytes(buf.Bytes(), &challenge); err != nil {
					return fmt.Errorf("error parsing template")
				}
				challenges = append(challenges, challenge)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return challenges, nil
}
