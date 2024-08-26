package gzcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

type GZ struct {
	api *gzapi.GZAPI
}

func Init() (*GZ, error) {
	config, err := GetConfig(&gzapi.GZAPI{})
	if err != nil {
		return nil, fmt.Errorf("error getting the config")
	}
	api, err := gzapi.Init(config.Url, &config.Creds)
	if err != nil {
		log.Error("Failed to login, try to register the account")
		if api, err = gzapi.Register(config.Url, &gzapi.RegisterForm{
			Email:    "admin@localhost",
			Username: config.Creds.Username,
			Password: config.Creds.Password,
		}); err != nil {
			log.Error("failed registering the account")
		}

		cmd := exec.Command(
			"sudo", "docker", "compose", "exec", "-T", "db", "psql",
			"--user", "postgres",
			"-d", "gzctf",
			"-c", fmt.Sprintf(`UPDATE "AspNetUsers" SET "Role"=3 WHERE "UserName"='%s';`, config.Creds.Username),
		)
		cmd.Dir, _ = os.Getwd()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Error("failed to change user role locally, please run manually")
			return nil, err
		}
	}
	return &GZ{
		api: api,
	}, nil
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
	games, err := gz.api.GetGames()
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
	config, err := GetConfig(gz.api)
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
	config, err := GetConfig(gz.api)
	if err != nil {
		return err
	}

	challengesConf, err := GetChallengesYaml()
	if err != nil {
		return err
	}

	games, err := gz.api.GetGames()
	if err != nil {
		return err
	}

	currentGame := findCurrentGame(games, config.Event.Title, gz.api)

	if currentGame == nil {
		DeleteCache("config")
		return gz.Sync()
	}

	err = updateGameIfNeeded(config, currentGame, gz.api)
	if err != nil {
		return err
	}

	err = validateChallenges(challengesConf)
	if err != nil {
		return err
	}

	config.Event.CS = gz.api
	challenges, err := config.Event.GetChallenges()
	if err != nil {
		return err
	}

	for _, challengeConf := range challengesConf {
		if err := syncChallenge(config, challengeConf, challenges, gz.api); err != nil {
			return err
		}
	}
	return nil
}
