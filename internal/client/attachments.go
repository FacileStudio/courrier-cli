package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// maxAttachmentBytes bounds one downloaded attachment, matching the 25 MB the
// instance accepts on the way in.
const maxAttachmentBytes = 25 << 20

// DownloadAttachment fetches the raw bytes of one attachment.
func (c *Client) DownloadAttachment(ctx context.Context, accountID, emailID, attachmentID int64) ([]byte, error) {
	path := mailPath(accountID, "/emails/"+strconv.FormatInt(emailID, 10)+
		"/attachments/"+strconv.FormatInt(attachmentID, 10)+"/download")
	request, err := c.newRequest(ctx, http.MethodGet, path, "", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s — %w", c.BaseURL, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, decodeError(raw, response.StatusCode)
	}
	if len(raw) > maxAttachmentBytes {
		return nil, fmt.Errorf("the attachment is larger than %d MB", maxAttachmentBytes>>20)
	}
	return raw, nil
}
