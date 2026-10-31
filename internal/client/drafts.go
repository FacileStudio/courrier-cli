package client

import (
	"context"
	"net/http"
	"strconv"
)

// SaveDraft stores a message in the account's drafts folder without sending it
// and returns the new draft's email id. With attachments the request is
// multipart, as for Send, and the instance keeps the files in the draft.
func (c *Client) SaveDraft(ctx context.Context, accountID int64, req SendRequest) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	path := mailPath(accountID, "/drafts")
	if len(req.Attachments) > 0 {
		err := c.multipart(ctx, http.MethodPost, path, req, &out)
		return out.ID, err
	}
	err := c.do(ctx, http.MethodPost, path, req.payload(), &out)
	return out.ID, err
}

// UpdateDraft replaces a draft's content in place and returns its id, which
// does not change. The fields of req replace the stored ones whole, so a caller
// that wants to keep a field sends it again. Attachments already on the draft
// stay unless their ids are in removeAttachments; the files in req.Attachments
// are added.
func (c *Client) UpdateDraft(ctx context.Context, accountID, emailID int64, req SendRequest, removeAttachments []int64) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	path := mailPath(accountID, "/drafts/"+strconv.FormatInt(emailID, 10))
	if len(req.Attachments) > 0 {
		req.RemoveAttachments = removeAttachments
		err := c.multipart(ctx, http.MethodPut, path, req, &out)
		return out.ID, err
	}
	body := struct {
		sendPayload
		RemoveAttachments []int64 `json:"remove_attachments"`
	}{req.payload(), removeAttachments}
	err := c.do(ctx, http.MethodPut, path, body, &out)
	return out.ID, err
}

// DeleteDraft removes a draft for good, from the instance and from the
// account's drafts mailbox.
func (c *Client) DeleteDraft(ctx context.Context, accountID, emailID int64) error {
	path := mailPath(accountID, "/drafts/"+strconv.FormatInt(emailID, 10))
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
