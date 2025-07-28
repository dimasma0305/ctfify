package gzapi

import (
	"encoding/json"
	"fmt"

	"github.com/dimasma0305/ctfify/function/log"
)

type Attachment struct {
	Id          int    `json:"id"`
	Type        string `json:"type"`
	Url         string `json:"url"`
	FileSize    int    `json:"fileSize"`
	GameId      int    `json:"-"`
	ChallengeId int    `json:"-"`
	CS          *GZAPI `json:"-" yaml:"-"`
}

func (a *Attachment) Delete() error {
	if a.CS == nil {
		return fmt.Errorf("GZAPI client is not initialized")
	}
	return a.CS.delete(fmt.Sprintf("/api/edit/games/%d/challenges/%d/attachment/%d", a.GameId, a.ChallengeId, a.Id), nil)
}

type CreateAttachmentForm struct {
	AttachmentType string `json:"attachmentType"`
	FileHash       string `json:"fileHash,omitempty"`
	RemoteUrl      string `json:"remoteUrl,omitempty"`
}

func (c *Challenge) CreateAttachment(attachment CreateAttachmentForm) error {
	if c.CS == nil {
		return fmt.Errorf("GZAPI client is not initialized")
	}

	// Debug: Log the attachment data being sent
	attachmentJSON, _ := json.Marshal(attachment)
	log.DebugH3("Creating attachment for challenge %s (ID: %d): %s", c.Title, c.Id, string(attachmentJSON))

	// Validate attachment data
	if attachment.AttachmentType == "" {
		return fmt.Errorf("attachment type is required")
	}

	if attachment.AttachmentType == "Local" && attachment.FileHash == "" {
		return fmt.Errorf("file hash is required for local attachments")
	}

	if attachment.AttachmentType == "Remote" && attachment.RemoteUrl == "" {
		return fmt.Errorf("remote URL is required for remote attachments")
	}

	err := c.CS.post(fmt.Sprintf("/api/edit/games/%d/challenges/%d/attachment", c.GameId, c.Id), attachment, nil)
	if err != nil {
		log.Error("Failed to create attachment for challenge %s: %v", c.Title, err)
		return fmt.Errorf("attachment creation failed for %s: %w", c.Title, err)
	}

	log.DebugH3("Successfully created attachment for challenge %s", c.Title)

	c.Attachment = &Attachment{
		Type: attachment.AttachmentType,
		Url:  attachment.FileHash,
	}
	return nil
}
