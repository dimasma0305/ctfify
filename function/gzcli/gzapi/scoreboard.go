package gzapi

import (
	"fmt"
)

type ScoreboardChallenge struct {
	Score int    `json:"score"`
	Tag   string `json:"tag"`
	Title string `json:"title"`
}

type ScoreboardItem struct {
	Name  string `json:"name"`
	Rank  int    `json:"rank"`
	Score int    `json:"score"`
}

type Scoreboard struct {
	Challenges map[string][]ScoreboardChallenge `json:"challenges"`
	Items      []ScoreboardItem                 `json:"items"`
}

func (g *Game) GetScoreboard() (*Scoreboard, error) {
	var scoreboard Scoreboard
	err := client.get(fmt.Sprintf("/api/game/%d/scoreboard", g.Id), &scoreboard)
	if err != nil {
		return nil, err
	}
	return &scoreboard, nil
}
