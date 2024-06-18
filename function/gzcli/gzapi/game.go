package gzapi

import (
	"fmt"
	"time"
)

type Game struct {
	Id                   int       `json:"id" yaml:"id"`
	Title                string    `json:"title" yaml:"title"`
	Hidden               bool      `json:"hidden" yaml:"hidden"`
	Summary              string    `json:"summary" yaml:"summary"`
	Content              string    `json:"content" yaml:"content"`
	AcceptWithoutReview  bool      `json:"acceptWithoutReview" yaml:"acceptWithoutReview"`
	WriteupRequired      bool      `json:"writeupRequired" yaml:"writeupRequired"`
	InviteCode           string    `json:"inviteCode,omitempty" yaml:"inviteCode,omitempty"`
	Organizations        []string  `json:"organizations,omitempty" yaml:"organizations,omitempty"`
	TeamMemberCountLimit int       `json:"teamMemberCountLimit" yaml:"teamMemberCountLimit"`
	ContainerCountLimit  int       `json:"containerCountLimit" yaml:"containerCountLimit"`
	Poster               string    `json:"poster,omitempty" yaml:"poster,omitempty"`
	PublicKey            string    `json:"publicKey" yaml:"publicKey"`
	PracticeMode         bool      `json:"practiceMode" yaml:"practiceMode"`
	Start                time.Time `json:"start" yaml:"start"`
	End                  time.Time `json:"end" yaml:"end"`
	WriteupDeadline      time.Time `json:"writeupDeadline,omitempty" yaml:"writeupDeadline,omitempty"`
	WriteupNote          string    `json:"writeupNote" yaml:"writeupNote"`
	BloodBonus           int       `json:"bloodBonus" yaml:"bloodBonus"`
}

func (cs *gzapi) GetGames() ([]Game, error) {
	var data struct {
		Data []Game `json:"data"`
	}
	if err := cs.get("/api/edit/games?count=9999&skip=0", &data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

func (cs *gzapi) GetGame(id int) (*Game, error) {
	var data *Game
	if err := cs.get(fmt.Sprintf("/api/edit/games/%d", id), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (g *Game) Delete() error {
	return client.delete(fmt.Sprintf("/api/edit/games/%d", g.Id), nil)
}

func (g *Game) Update(game *Game) error {
	return client.put(fmt.Sprintf("/api/edit/games/%d", g.Id), game, nil)
}

func (g *Game) UploadPoster(poster string) (string, error) {
	var path string
	if err := client.putMultiPart(fmt.Sprintf("/api/edit/games/%d/poster", g.Id), poster, &path); err != nil {
		return "", err
	}
	return path, nil
}

type CreateGameForm struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (cs *gzapi) CreateGame(game CreateGameForm) (*Game, error) {
	var data *Game
	game.Start = game.Start.UTC()
	game.End = game.End.UTC()
	if err := cs.post("/api/edit/games", game, &data); err != nil {
		return nil, err
	}
	return data, nil
}
