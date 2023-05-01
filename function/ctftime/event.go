package ctftime

import (
	"encoding/json"
	"log"
	"strconv"
	"time"
)

type Event struct {
	Organizers []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"organizers"`
	Onsite        bool    `json:"onsite"`
	Finish        string  `json:"finish"`
	Description   string  `json:"description"`
	Weight        float64 `json:"weight"`
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	IsVotableNow  bool    `json:"is_votable_now"`
	Restrictions  string  `json:"restrictions"`
	Format        string  `json:"format"`
	Start         string  `json:"start"`
	Participants  int     `json:"participants"`
	CTFTimeURL    string  `json:"ctftime_url"`
	Location      string  `json:"location"`
	LiveFeed      string  `json:"live_feed"`
	PublicVotable bool    `json:"public_votable"`
	Duration      struct {
		Hours int `json:"hours"`
		Days  int `json:"days"`
	} `json:"duration"`
	Logo     string `json:"logo"`
	FormatID int    `json:"format_id"`
	ID       int    `json:"id"`
	CTFID    int    `json:"ctf_id"`
}

func (ca *ctftimeApi) GetEventByTimeStamp(limit int, start int, finish int) (Events, error) {
	var events []*Event
	res, err := api.client.R().
		SetQueryParams(map[string]string{
			"limit":  strconv.Itoa(limit),
			"start":  strconv.Itoa(start),
			"finish": strconv.Itoa(finish),
		}).
		Get(api.url + "/events/")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if err := json.Unmarshal(res.Bytes(), &events); err != nil {
		return nil, err
	}
	return events, nil
}

// Get events by date
// as example "January 2, 2006"
func (ca *ctftimeApi) GetEventsByDate(limit int, start string, finish string) (Events, error) {
	var (
		dateStart  = dateToTimestamp(start)
		dateFinish = dateToTimestamp(finish)
	)
	res, err := ca.GetEventByTimeStamp(limit, dateStart, dateFinish)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// transte date to timestamp
func dateToTimestamp(dateString string) int {
	layout := "January 2, 2006"
	t, err := time.Parse(layout, dateString)
	if err != nil {
		log.Fatal(err)
	}
	return int(t.Unix())
}
