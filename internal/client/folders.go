package client

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// MoveTarget names the folder a move goes to: an ID for any folder, or a Type
// for a system folder (inbox, sent, drafts, trash, junk, archive). Exactly one
// is set.
type MoveTarget struct {
	FolderID   int64
	FolderType string
}

// MoveEmails moves messages into a folder. The instance moves them on the IMAP
// server first and keeps the message ids. Rate limited to thirty calls a
// minute; a batch is at most maxBulkIDs messages.
func (c *Client) MoveEmails(ctx context.Context, accountID int64, ids []int64, target MoveTarget) error {
	if len(ids) == 0 {
		return fmt.Errorf("a move needs at least one email id")
	}
	if len(ids) > maxBulkIDs {
		return fmt.Errorf("a move takes at most %d email ids, got %d", maxBulkIDs, len(ids))
	}
	body := struct {
		EmailIDs   []int64 `json:"email_ids"`
		FolderID   *int64  `json:"folder_id,omitempty"`
		FolderType string  `json:"folder_type,omitempty"`
	}{EmailIDs: ids, FolderType: target.FolderType}
	if target.FolderID != 0 {
		body.FolderID = &target.FolderID
	}
	return c.do(ctx, http.MethodPost, mailPath(accountID, "/emails/move"), body, nil)
}

// CreateFolder creates a top-level folder on the IMAP server and returns it.
func (c *Client) CreateFolder(ctx context.Context, accountID int64, name string) (Folder, error) {
	var out Folder
	body := struct {
		Name string `json:"name"`
	}{name}
	err := c.do(ctx, http.MethodPost, mailPath(accountID, "/folders"), body, &out)
	return out, err
}

// RenameFolder renames a custom folder and returns it. System folders are
// refused by the instance.
func (c *Client) RenameFolder(ctx context.Context, accountID, folderID int64, name string) (Folder, error) {
	var out Folder
	body := struct {
		Name string `json:"name"`
	}{name}
	path := mailPath(accountID, "/folders/"+strconv.FormatInt(folderID, 10))
	err := c.do(ctx, http.MethodPatch, path, body, &out)
	return out, err
}

// DeleteFolder deletes a custom folder. A folder that still holds messages is
// refused with a 409 unless force is set, because IMAP deletes the messages
// with the mailbox.
func (c *Client) DeleteFolder(ctx context.Context, accountID, folderID int64, force bool) error {
	path := mailPath(accountID, "/folders/"+strconv.FormatInt(folderID, 10))
	if force {
		path += "?force=true"
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
