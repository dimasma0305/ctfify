package gzcli

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
)

const (
	CONFIG_FILE = "conf.yaml"
)

var (
	CHALLENGE_CATEGORY = []string{
		"Misc", "Crypto", "Pwn",
		"Web", "Reverse", "Blockchain",
		"Forensics", "Hardware", "Mobile", "PPC",
		"Osint",
	}
)

func GetConfig(api *gzapi.GZAPI) (*Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var config Config
	confPath := filepath.Join(dir, CONFIG_FILE)
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

func GetChallengesYaml() ([]ChallengeYaml, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	var challenges []ChallengeYaml
	ChallengeFilePattern := regexp.MustCompile(`challenge\.(yaml|yml)$`)
	for _, category := range CHALLENGE_CATEGORY {
		categoryPath := filepath.Join(dir, category)
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
				challenge.Tag = category
				challenge.Cwd = filepath.Dir(path)
				if category == "Osint" {
					challenge.Tag = "Misc"
					challenge.Name = challenge.Name + " - Osint"
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
