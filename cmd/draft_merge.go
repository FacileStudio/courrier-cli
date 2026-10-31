package cmd

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

// loadDraft fetches a message and refuses anything that is not in the drafts
// folder, so update can never delete a real message.
func loadDraft(ctx context.Context, api *client.Client, accountID, id int64) (client.Email, error) {
	email, err := api.Email(ctx, accountID, id)
	if err != nil {
		return email, err
	}
	folders, err := api.Folders(ctx, accountID)
	if err != nil {
		return email, err
	}
	folder, ok := folderByID(folders, email.FolderID)
	if !ok || folder.Type != "drafts" {
		return email, errors.New("email " + strconv.FormatInt(id, 10) + " is not a draft — only drafts can be changed this way")
	}
	return email, nil
}

// mergeDraft overlays the flags the caller passed onto the stored draft.
func mergeDraft(cmd *cobra.Command, f *composeFlags, stored client.Email, text, html *string) client.SendRequest {
	req := client.SendRequest{
		To:          addressList(stored.ToAddresses),
		Cc:          addressList(stored.CcAddresses),
		Bcc:         addressList(stored.BccAddresses),
		Subject:     stored.Subject,
		Body:        stored.BodyText,
		BodyHTML:    stored.BodyHTML,
		InReplyTo:   stored.InReplyTo,
		References:  strings.Fields(stored.References),
		Attachments: f.attach,
	}
	changed := cmd.Flags().Changed
	if changed("to") {
		req.To = f.to
	}
	if changed("cc") {
		req.Cc = f.cc
	}
	if changed("bcc") {
		req.Bcc = f.bcc
	}
	if changed("subject") {
		req.Subject = f.subject
	}
	if changed("in-reply-to") {
		req.InReplyTo = f.inReplyTo
	}
	if changed("references") {
		req.References = f.references
	}
	if text != nil {
		req.Body = *text
	}
	if html != nil {
		req.BodyHTML = *html
	}
	return req
}

// addressList flattens parsed addresses to the bare emails the API accepts.
func addressList(addresses []client.Address) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, address.Email)
	}
	return out
}
