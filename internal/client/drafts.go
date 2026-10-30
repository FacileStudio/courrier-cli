package client

import (
	"context"
	"net/http"
	"strconv"
)

// SaveDraft stores a message in the account's drafts folder without sending it
// and returns the new draft's email id. The server takes JSON only, so a draft
// cannot carry attachments, and a draft cannot be edited in place: changing one
// means saving a new draft and deleting the old.
func (c *Client) SaveDraft(ctx context.Context, accountID int64, req SendRequest) (int64, error) {
	body := sendPayload{
		To:         req.To,
		Cc:         req.Cc,
		Subject:    req.Subject,
		Body:       req.Body,
		BodyHTML:   req.BodyHTML,
		InReplyTo:  req.InReplyTo,
		References: req.References,
	}
	var out struct {
		ID int64 `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, mailPath(accountID, "/drafts"), body, &out)
	return out.ID, err
}

// DeleteDraft removes a draft for good.
func (c *Client) DeleteDraft(ctx context.Context, accountID, emailID int64) error {
	path := mailPath(accountID, "/drafts/"+strconv.FormatInt(emailID, 10))
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
