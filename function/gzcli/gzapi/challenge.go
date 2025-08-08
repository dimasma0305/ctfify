package gzapi

import (
	"fmt"
	"sync"
)

type Challenge struct {
	Id                   int         `json:"id" yaml:"id"`
	Title                string      `json:"title" yaml:"title"`
	Content              string      `json:"content" yaml:"content"`
	Category             string      `json:"category" yaml:"category"`
	Type                 string      `json:"type" yaml:"type"`
	Hints                []string    `json:"hints" yaml:"hints"`
	FlagTemplate         string      `json:"flagTemplate" yaml:"flagTemplate"`
	IsEnabled            *bool       `json:"isEnabled,omitempty" yaml:"isEnabled,omitempty"`
	AcceptedCount        int         `json:"acceptedCount" yaml:"acceptedCount"`
	FileName             string      `json:"fileName" yaml:"fileName"`
	Attachment           *Attachment `json:"attachment" yaml:"attachment"`
	TestContainer        interface{} `json:"testContainer" yaml:"testContainer"`
	Flags                []Flag      `json:"flags" yaml:"flags"`
	ContainerImage       string      `json:"containerImage" yaml:"containerImage"`
	MemoryLimit          int         `json:"memoryLimit" yaml:"memoryLimit"`
	CpuCount             int         `json:"cpuCount" yaml:"cpuCount"`
	StorageLimit         int         `json:"storageLimit" yaml:"storageLimit"`
	ContainerExposePort  int         `json:"containerExposePort" yaml:"containerExposePort"`
	EnableTrafficCapture bool        `json:"enableTrafficCapture" yaml:"enableTrafficCapture"`
	OriginalScore        int         `json:"originalScore" yaml:"originalScore"`
	MinScoreRate         float64     `json:"minScoreRate" yaml:"minScoreRate"`
	Difficulty           float64     `json:"difficulty" yaml:"difficulty"`
	GameId               int         `json:"-" yaml:"gameId"`
	CS                   *GZAPI      `json:"-" yaml:"-"`
}

func (c *Challenge) Delete() error {
	if c.CS == nil {
		return fmt.Errorf("GZAPI client is not initialized")
	}
	return c.CS.delete(fmt.Sprintf("/api/edit/games/%d/challenges/%d", c.GameId, c.Id), nil)
}

func (c *Challenge) Update(challenge Challenge) (*Challenge, error) {
	if c.CS == nil {
		return nil, fmt.Errorf("GZAPI client is not initialized")
	}
	if err := c.CS.put(fmt.Sprintf("/api/edit/games/%d/challenges/%d", c.GameId, c.Id), &challenge, nil); err != nil {
		return nil, err
	}
	return &challenge, nil
}

func (c *Challenge) Refresh() (*Challenge, error) {
	if c.CS == nil {
		return nil, fmt.Errorf("GZAPI client is not initialized")
	}
	var data Challenge
	if err := c.CS.get(fmt.Sprintf("/api/edit/games/%d/challenges/%d", c.GameId, c.Id), &data); err != nil {
		return nil, err
	}
	data.GameId = c.GameId
	data.CS = c.CS
	return &data, nil
}

type CreateChallengeForm struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Tag      string `json:"tag"`
	Type     string `json:"type"`
}

func (g *Game) CreateChallenge(challenge CreateChallengeForm) (*Challenge, error) {
	if g.CS == nil {
		return nil, fmt.Errorf("GZAPI client is not initialized")
	}

	var data *Challenge
	if err := g.CS.post(fmt.Sprintf("/api/edit/games/%d/challenges", g.Id), challenge, &data); err != nil {
		return nil, err
	}
	data.GameId = g.Id
	data.CS = g.CS
	return data, nil
}

func (g *Game) GetChallenges() ([]Challenge, error) {
	if g.CS == nil {
		return nil, fmt.Errorf("GZAPI client is not initialized")
	}

	var tmp []Challenge
	var data []Challenge
	if err := g.CS.get(fmt.Sprintf("/api/edit/games/%d/challenges", g.Id), &tmp); err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := range tmp {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var c Challenge
			if err := g.CS.get(fmt.Sprintf("/api/edit/games/%d/challenges/%d", g.Id, tmp[i].Id), &c); err != nil {
				return
			}
			c.GameId = g.Id
			c.CS = g.CS

			mu.Lock()
			data = append(data, c)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return data, nil
}

func (g *Game) GetChallenge(name string) (*Challenge, error) {
	var data []Challenge
	if err := g.CS.get(fmt.Sprintf("/api/edit/games/%d/challenges", g.Id), &data); err != nil {
		return nil, err
	}
	var challenge *Challenge
	for _, v := range data {
		if v.Title == name {
			challenge = &v
		}
	}
	if challenge == nil {
		return nil, fmt.Errorf("challenge not found")
	}
	if err := g.CS.get(fmt.Sprintf("/api/edit/games/%d/challenges/%d", g.Id, challenge.Id), &challenge); err != nil {
		return nil, err
	}
	challenge.GameId = g.Id
	challenge.CS = g.CS
	return challenge, nil
}
