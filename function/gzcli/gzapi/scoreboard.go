package gzapi

import (
	"fmt"
)

type ScoreboardChallenge struct {
	Score    int    `json:"score"`
	Category string `json:"category"`
	Title    string `json:"title"`
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
	err := g.CS.get(fmt.Sprintf("/api/game/%d/scoreboard", g.Id), &scoreboard)
	if err != nil {
		return nil, err
	}
	return &scoreboard, nil
}
