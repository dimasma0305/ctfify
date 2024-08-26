package gzapi

import "fmt"

type Flag struct {
	Id          int        `json:"id"`
	Flag        string     `json:"flag"`
	Attachment  Attachment `json:"attachment"`
	GameId      int        `json:"-"`
	ChallengeId int        `json:"-"`
	CS          *GZAPI     `json:"-"`
}

func (f *Flag) Delete() error {
	return f.CS.delete(fmt.Sprintf("/api/edit/games/%d/challenges/%d/flags/%d", f.GameId, f.ChallengeId, f.Id), nil)
}

type CreateFlagForm struct {
	Flag string `json:"flag"`
}

func (c *Challenge) CreateFlag(flag CreateFlagForm) error {
	flags := []CreateFlagForm{flag}
	if err := c.CS.post(fmt.Sprintf("/api/edit/games/%d/challenges/%d/flags", c.GameId, c.Id), flags, nil); err != nil {
		return err
	}
	return nil
}

func (c *Challenge) GetFlags() []Flag {
	for i := range c.Flags {
		c.Flags[i].CS = c.CS
		c.Flags[i].GameId = c.GameId
		c.Flags[i].ChallengeId = c.Id
	}
	return c.Flags
}
