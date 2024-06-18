package gzapi

import "fmt"

type Attachment struct {
	Id          int    `json:"id"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	FileSize    int    `json:"fileSize"`
	GameId      int    `json:"-"`
	ChallengeId int    `json:"-"`
}

func (a *Attachment) Delete() error {
	return client.delete(fmt.Sprintf("/api/edit/games/%d/challenges/%d/attachment/%d", a.GameId, a.ChallengeId, a.Id), nil)
}

type CreateAttachmentForm struct {
	AttachmentType string `json:"attachmentType"`
	FileHash       string `json:"fileHash,omitempty"`
	RemoteUrl      string `json:"remoteUrl,omitempty"`
}

func (c *Challenge) CreateAttachment(attachment CreateAttachmentForm) error {
	return client.post(fmt.Sprintf("/api/edit/games/%d/challenges/%d/attachment", c.GameId, c.Id), attachment, nil)
}
