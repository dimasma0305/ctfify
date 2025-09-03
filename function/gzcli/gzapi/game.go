package gzapi

import (
	"encoding/json"
	"fmt"
	"time"
)

type Game struct {
	Id                   int        `json:"id" yaml:"id"`
	Title                string     `json:"title" yaml:"title"`
	Hidden               bool       `json:"hidden" yaml:"hidden"`
	Summary              string     `json:"summary" yaml:"summary"`
	Content              string     `json:"content" yaml:"content"`
	AcceptWithoutReview  bool       `json:"acceptWithoutReview" yaml:"acceptWithoutReview"`
	WriteupRequired      bool       `json:"writeupRequired" yaml:"writeupRequired"`
	InviteCode           string     `json:"inviteCode,omitempty" yaml:"inviteCode,omitempty"`
	Organizations        []string   `json:"organizations,omitempty" yaml:"organizations,omitempty"`
	TeamMemberCountLimit int        `json:"teamMemberCountLimit" yaml:"teamMemberCountLimit"`
	ContainerCountLimit  int        `json:"containerCountLimit" yaml:"containerCountLimit"`
	Poster               string     `json:"poster,omitempty" yaml:"poster,omitempty"`
	PublicKey            string     `json:"publicKey" yaml:"publicKey"`
	PracticeMode         bool       `json:"practiceMode" yaml:"practiceMode"`
	Start                CustomTime `json:"start" yaml:"start"`
	End                  CustomTime `json:"end" yaml:"end"`
	WriteupDeadline      CustomTime `json:"writeupDeadline,omitempty" yaml:"writeupDeadline,omitempty"`
	WriteupNote          string     `json:"writeupNote" yaml:"writeupNote"`
	BloodBonus           int        `json:"bloodBonus" yaml:"bloodBonus"`
	CS                   *GZAPI     `json:"-" yaml:"-"`
}

type CustomTime struct {
	time.Time
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	// The input comes as a number (milliseconds since epoch).
	var ms int64
	if err := json.Unmarshal(b, &ms); err != nil {
		// Try to parse as a string.
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("invalid time format: %s", string(b))
		}
		// Parse the string as a time.
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("invalid time format: %s", s)
		}
		ct.Time = t
		return nil
	}

	// Convert milliseconds to seconds and set the time.
	ct.Time = time.Unix(0, ms*int64(time.Millisecond))
	return nil
}

func (cs *GZAPI) GetGames() ([]*Game, error) {
	var data struct {
		Data []*Game `json:"data"`
	}
	if err := cs.get("/api/edit/games?count=100&skip=0", &data); err != nil {
		return nil, err
	}
	for _, game := range data.Data {
		game.CS = cs
	}
	return data.Data, nil
}

func (cs *GZAPI) GetGameById(id int) (*Game, error) {
	var data *Game
	if err := cs.get(fmt.Sprintf("/api/edit/games/%d", id), &data); err != nil {
		return nil, err
	}
	data.CS = cs
	return data, nil
}

func (cs *GZAPI) GetGameByTitle(title string) (*Game, error) {
	var games []*Game
	games, err := cs.GetGames()
	if err != nil {
		return nil, err
	}
	for _, game := range games {
		if game.Title == title {
			return game, nil
		}
	}
	return nil, fmt.Errorf("game not found")
}

func (g *Game) Delete() error {
	return g.CS.delete(fmt.Sprintf("/api/edit/games/%d", g.Id), nil)
}

func (g *Game) Update(game *Game) error {
	// Create a copy to avoid modifying the original
	gameCopy := *game

	// Convert all time fields to UTC to avoid PostgreSQL timezone issues
	gameCopy.Start.Time = gameCopy.Start.Time.UTC()
	gameCopy.End.Time = gameCopy.End.Time.UTC()
	if !gameCopy.WriteupDeadline.Time.IsZero() {
		gameCopy.WriteupDeadline.Time = gameCopy.WriteupDeadline.Time.UTC()
	}

	return g.CS.put(fmt.Sprintf("/api/edit/games/%d", g.Id), &gameCopy, nil)
}

func (g *Game) UploadPoster(poster string) (string, error) {
	var path string
	if err := g.CS.putMultiPart(fmt.Sprintf("/api/edit/games/%d/poster", g.Id), poster, &path); err != nil {
		return "", err
	}
	return path, nil
}

type CreateGameForm struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (cs *GZAPI) CreateGame(game CreateGameForm) (*Game, error) {
	var data *Game
	game.Start = game.Start.UTC()
	game.End = game.End.UTC()
	if err := cs.post("/api/edit/games", game, &data); err != nil {
		return nil, err
	}
	data.CS = cs
	return data, nil
}

type GameJoinModel struct {
	TeamId     int    `json:"teamId"`
	Division   string `json:"division,omitempty"`
	InviteCode string `json:"inviteCode,omitempty"`
}

func (g *Game) JoinGame(teamId int, division string, inviteCode string) error {
	joinModel := &GameJoinModel{
		TeamId:     teamId,
		Division:   division,
		InviteCode: inviteCode,
	}
	return g.CS.post(fmt.Sprintf("/api/game/%d", g.Id), joinModel, nil)
}
