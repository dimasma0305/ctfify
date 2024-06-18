package gzcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regexp"

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
	Name        string    `yaml:"name"`
	Author      string    `yaml:"author"`
	Description string    `yaml:"description"`
	Flags       []string  `yaml:"flags"`
	Value       int       `yaml:"value"`
	Provide     *string   `yaml:"provide,omitempty"`
	Visible     *bool     `yaml:"visible"`
	Type        string    `yaml:"type"`
	Hints       []string  `yaml:"hints"`
	Container   Container `yaml:"container"`
	Tag         string    `yaml:"-"`
	Cwd         string    `yaml:"-"`
}

func InitFolder() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	for _, category := range CHALLENGE_CATEGORY {
		categoryPath := filepath.Join(dir, category)
		if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
			if err := os.Mkdir(categoryPath, os.ModePerm); err != nil {
				return err
			}

			if _, err := os.Create(filepath.Join(categoryPath, ".gitkeep")); err != nil {
				return err
			}
		}
	}

	if err := copyAllEmbedFileIntoFolder("embeds/config", dir); err != nil {
		return err
	}

	return nil
}

func RemoveAllEvent() error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	client, err := gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		return err
	}

	games, err := client.GetGames()
	if err != nil {
		return err
	}

	for _, game := range games {
		if err := game.Delete(); err != nil {
			return err
		}
	}

	return nil
}

func Sync() error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	challengesConf, err := GetChallengesYaml()
	if err != nil {
		return err
	}

	client, err := gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		return err
	}

	games, err := client.GetGames()
	if err != nil {
		return err
	}

	var currentGame *gzapi.Game
	for _, game := range games {
		if game.Title == config.Event.Title {
			currentGame = &game
			break
		}
	}

	if currentGame == nil {
		log.Info("Create new game")
		event := gzapi.CreateGameForm{
			Title: config.Event.Title,
			Start: config.Event.Start,
			End:   config.Event.End,
		}
		game, err := client.CreateGame(event)
		if err != nil {
			return err
		}
		if config.Event.Poster == "" {
			return fmt.Errorf("poster is required")
		}

		poster, err := createPosterIfNotExistOrDifferent(config.Event.Poster, game)
		if err != nil {
			return err
		}

		config.Event.Id = game.Id
		config.Event.PublicKey = game.PublicKey
		config.Event.Poster = poster
		if err := game.Update(&config.Event); err != nil {
			return err
		}
		if err := setCache("config", config); err != nil {
			return err
		}
	} else {
		poster, err := createPosterIfNotExistOrDifferent(config.Event.Poster, currentGame)
		if err != nil {
			return err
		}
		config.Event.Poster = poster
		if fmt.Sprintf("%v", config.Event) != fmt.Sprintf("%v", *currentGame) {
			log.Info("Updated %s game", config.Event.Title)

			config.Event.Id = currentGame.Id
			config.Event.PublicKey = currentGame.PublicKey

			if err := currentGame.Update(&config.Event); err != nil {
				return err
			}
			if err := setCache("config", config); err != nil {
				return err
			}
		}
	}
	for _, challengeConf := range challengesConf {
		if challengeConf.Type == "" {
			challengeConf.Type = "StaticAttachments"
		}

		if err := isGoodChallenge(challengeConf); err != nil {
			return err
		}
	}

	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return err
	}

	for _, challengeConf := range challengesConf {
		var challengeData *gzapi.Challenge
		if !isChallengeExist(challengeConf.Name, challenges) {
			log.Info("Create challenge %s", challengeConf.Name)
			challengeData, err = config.Event.CreateChallenge(gzapi.CreateChallengeForm{
				Title: challengeConf.Name,
				Tag:   challengeConf.Tag,
				Type:  challengeConf.Type,
			})
			if err != nil {
				return fmt.Errorf("create challenge %s: %v", challengeConf.Name, err)
			}
		} else {
			log.Info("Update challenge %s", challengeConf.Name)
			if err := GetCache(challengeConf.Tag+"/"+challengeConf.Name+"/challenge", &challengeData); err != nil {
				challengeData, err = config.Event.GetChallenge(challengeConf.Name)
				if err != nil {
					return fmt.Errorf("get challenge %s: %v", challengeConf.Name, err)
				}
			}
		}
		if challengeConf.Provide != nil {
			if regexp.MustCompile(`^http(s|)://`).MatchString(*challengeConf.Provide) {
				log.Info("Create remote attachment for %s", challengeConf.Name)
				if err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
					AttachmentType: "Remote",
					RemoteUrl:      *challengeConf.Provide,
				}); err != nil {
					return err
				}
			} else {
				log.Info("Create local attachment for %s", challengeConf.Name)
				zipFilename := hashString(*challengeConf.Provide) + ".zip"
				zipOutput := filepath.Join(challengeConf.Cwd, zipFilename)
				if info, err := os.Stat(filepath.Join(challengeConf.Cwd, *challengeConf.Provide)); err != nil || info.IsDir() {
					log.Info("Zip attachment for %s", challengeConf.Name)
					zipInput := filepath.Join(challengeConf.Cwd, *challengeConf.Provide)
					if err := zipSource(zipInput, zipOutput); err != nil {
						return err
					}
					challengeConf.Provide = &zipFilename
				}
				fileinfo, err := createAssetsIfNotExistOrDifferent(filepath.Join(challengeConf.Cwd, *challengeConf.Provide))
				if err != nil {
					return err
				}

				if challengeData.Attachment != nil && strings.Contains(challengeData.Attachment.Url, fileinfo.Hash) {
					log.Info("Attachment for %s is the same...", challengeConf.Name)
				} else {
					log.Info("Update attachment for %s", challengeConf.Name)
					if err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
						AttachmentType: "Local",
						FileHash:       fileinfo.Hash,
					}); err != nil {
						return err
					}
				}
				os.Remove(zipOutput)
			}
		} else {
			if challengeData.Attachment != nil {
				log.Info("Delete attachment for %s", challengeConf.Name)
				if err := challengeData.CreateAttachment(gzapi.CreateAttachmentForm{
					AttachmentType: "None",
				}); err != nil {
					return err
				}
			}
		}

		challengeData, err = config.Event.GetChallenge(challengeConf.Name)
		if err != nil {
			return err
		}

		for _, flag := range challengeData.Flags {
			if !isExistInArray(flag.Flag, challengeConf.Flags) {
				flag.GameId = config.Event.Id
				flag.ChallengeId = challengeData.Id
				if err := flag.Delete(); err != nil {
					return err
				}
			}
		}

		for _, flag := range challengeConf.Flags {
			if !isFlagExist(flag, challengeData.Flags) {
				if err := challengeData.CreateFlag(gzapi.CreateFlagForm{
					Flag: flag,
				}); err != nil {
					return err
				}
			}
		}

		mergeChallengeData(&challengeConf, challengeData)

		if !isConfigEdited(&challengeConf, challengeData) {
			log.Info("Challenge %s is the same...", challengeConf.Name)
			continue
		}

		if err := challengeData.Update(*challengeData); err != nil {
			return err
		}

		if err := setCache(challengeData.Tag+"/"+challengeConf.Name+"/challenge", challengeData); err != nil {
			return err
		}
	}
	return nil
}
