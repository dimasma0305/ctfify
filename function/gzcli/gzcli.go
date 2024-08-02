package gzcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
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
	Tag         string            `yaml:"-"`
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

type GZ struct{}

var api *gzapi.API

func Init() (*GZ, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}
	api, err = gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		return nil, err
	}
	return &GZ{}, nil
}

func New() *GZ {
	return &GZ{}
}

func (gz *GZ) InitFolder() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	for _, category := range CHALLENGE_CATEGORY {
		categoryPath := filepath.Join(dir, category)
		if err := createCategoryFolder(categoryPath); err != nil {
			return fmt.Errorf("create category folder: %v", err)
		}
	}

	return copyAllEmbedFileIntoFolder("embeds/config", dir)
}

func createCategoryFolder(categoryPath string) error {
	if _, err := os.Stat(categoryPath); os.IsNotExist(err) {
		if err := os.Mkdir(categoryPath, os.ModePerm); err != nil {
			return err
		}
		_, err = os.Create(filepath.Join(categoryPath, ".gitkeep"))
		return err
	}
	return nil
}

func (gz *GZ) RemoveAllEvent() error {
	games, err := api.GetGames()
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

func (gz *GZ) Scoreboard2CTFTimeFeed() (*CTFTimeFeed, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, err
	}
	scoreboard, err := config.Event.GetScoreboard()
	if err != nil {
		return nil, fmt.Errorf("get scoreboard: %v", err)
	}

	var ctfTimeFeed CTFTimeFeed
	for _, item := range scoreboard.Items {
		ctfTimeFeed.Standings = append(ctfTimeFeed.Standings, Standing{
			Pos:   item.Rank,
			Team:  item.Name,
			Score: item.Score,
		})
	}

	for category, items := range scoreboard.Challenges {
		for _, item := range items {
			ctfTimeFeed.Tasks = append(ctfTimeFeed.Tasks, fmt.Sprintf("%s - %s", category, item.Title))
		}
	}
	return &ctfTimeFeed, nil
}

func (gz *GZ) RunScript(script string) error {
	challengesConf, err := GetChallengesYaml()
	if err != nil {
		return err
	}

	for _, challengeConf := range challengesConf {
		if _, ok := challengeConf.Scripts[script]; ok {
			if err := runScript(challengeConf, script); err != nil {
				return err
			}
		}
	}
	return nil
}

func (gz *GZ) Sync() error {
	config, err := GetConfig()
	if err != nil {
		return err
	}

	challengesConf, err := GetChallengesYaml()
	if err != nil {
		return err
	}

	games, err := api.GetGames()
	if err != nil {
		return err
	}

	currentGame := findCurrentGame(games, config.Event.Title)

	err = updateGameIfNeeded(config, currentGame)
	if err != nil {
		return err
	}

	err = validateChallenges(challengesConf)
	if err != nil {
		return err
	}

	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return err
	}

	for _, challengeConf := range challengesConf {
		if err := syncChallenge(config, challengeConf, challenges); err != nil {
			return err
		}
	}
	return nil
}
