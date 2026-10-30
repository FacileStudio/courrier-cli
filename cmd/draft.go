package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	draftFlags    composeFlags
	draftEditFlag composeFlags
	flagDraftYes  bool
)

var draftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Create, change and delete drafts",
	Long: `Manage drafts in the account's drafts folder.

Read drafts with courrier list drafts and courrier read --id <id>. The server
stores drafts from JSON only, so a draft carries no attachments. It cannot edit
a draft in place either: update saves a new draft and deletes the old one, so
the id changes.`,
}

var draftCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Save a message as a draft",
	Args:  cobra.NoArgs,
	RunE:  runDraftCreate,
}

var draftUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Change a draft, keeping the fields you do not pass",
	Args:  cobra.ExactArgs(1),
	RunE:  runDraftUpdate,
}

var draftDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a draft",
	Args:  cobra.ExactArgs(1),
	RunE:  runDraftDelete,
}

// init wires the draft commands and flags.
func init() {
	draftFlags.bind(draftCreateCmd, false)
	draftEditFlag.bind(draftUpdateCmd, false)
	draftDeleteCmd.Flags().BoolVar(&flagDraftYes, "yes", false, "Answer the confirmation with yes")
	draftCmd.AddCommand(draftCreateCmd, draftUpdateCmd, draftDeleteCmd)
}

// draftReport is the --json document for a draft write.
type draftReport struct {
	AccountID int64  `json:"account_id"`
	ID        int64  `json:"id"`
	Replaced  *int64 `json:"replaced,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// runDraftCreate saves the message exactly as the flags describe it.
func runDraftCreate(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	text, html, err := draftFlags.bodies(cmd)
	if err != nil {
		return err
	}
	if text, err = draftFlags.stdinBody(text, html); err != nil {
		return err
	}
	if err := checkAddresses(draftFlags.to, draftFlags.cc); err != nil {
		return err
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	id, err := api.SaveDraft(ctx, accountID, draftFlags.request(text, html))
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot save the draft — %w", err))
	}
	return reportDraft(draftReport{AccountID: accountID, ID: id}, "draft saved as id %d", id)
}

// runDraftUpdate merges the flags over the stored draft, saves the result and
// removes the original.
func runDraftUpdate(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	oldID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || oldID <= 0 {
		return fmt.Errorf("%q is not a draft id — ids are the numbers in `courrier list drafts`", args[0])
	}
	text, html, err := draftEditFlag.bodies(cmd)
	if err != nil {
		return err
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	stored, err := loadDraft(ctx, api, accountID, oldID)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	req := mergeDraft(cmd, &draftEditFlag, stored, text, html)
	if err := checkAddresses(req.To, req.Cc); err != nil {
		return err
	}
	newID, err := api.SaveDraft(ctx, accountID, req)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot save the changed draft — %w", err))
	}
	if err := api.DeleteDraft(ctx, accountID, oldID); err != nil {
		return fmt.Errorf("the changed draft is saved as id %d but the old draft %d could not be deleted — %w", newID, oldID, err)
	}
	return reportDraft(draftReport{AccountID: accountID, ID: newID, Replaced: &oldID}, "draft %d saved as id %d", oldID, newID)
}

// runDraftDelete removes one draft after the usual confirmation.
func runDraftDelete(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("%q is not a draft id — ids are the numbers in `courrier list drafts`", args[0])
	}
	if err := confirmDestructive("draft "+args[0], flagDraftYes); err != nil {
		return err
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	if err := api.DeleteDraft(ctx, accountID, id); err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot delete draft %d — %w", id, err))
	}
	return reportDraft(draftReport{AccountID: accountID, ID: id, Deleted: true}, "draft %d deleted", id)
}

// openAccount opens the session and resolves the account every account-scoped
// command acts on.
func openAccount(ctx context.Context) (*client.Client, int64, error) {
	api, cfg, err := session()
	if err != nil {
		return nil, 0, err
	}
	accountID, err := resolveAccount(ctx, api, cfg)
	if err != nil {
		return nil, 0, wrapInterrupt(ctx, err)
	}
	return api, accountID, nil
}

// reportDraft prints the outcome as JSON or one success line.
func reportDraft(report draftReport, format string, a ...any) error {
	if flagJSON {
		return ui.JSON(report)
	}
	ui.Success(format, a...)
	return nil
}

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
		To:         addressList(stored.ToAddresses),
		Cc:         addressList(stored.CcAddresses),
		Subject:    stored.Subject,
		Body:       stored.BodyText,
		BodyHTML:   stored.BodyHTML,
		InReplyTo:  stored.InReplyTo,
		References: strings.Fields(stored.References),
	}
	changed := cmd.Flags().Changed
	if changed("to") {
		req.To = f.to
	}
	if changed("cc") {
		req.Cc = f.cc
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
