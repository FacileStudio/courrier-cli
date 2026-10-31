package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	draftFlags    composeFlags
	draftEditFlag composeFlags
	flagDraftYes  bool
	flagDraftDrop []int64
)

var draftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Create, change and delete drafts",
	Long: `Manage drafts in the account's drafts folder.

Read drafts with courrier list drafts and courrier read --id <id>.

A draft takes the same fields as send, --bcc and --attach included. update
changes it in place: the id stays the same, fields you do not pass are kept, and
attachments already on the draft stay unless you name them with
--remove-attachment <attachment-id> (ids are in courrier read --id <id> --json).
delete removes the draft from the instance and from the drafts mailbox.`,
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
	draftFlags.bindAttach(draftCreateCmd)
	draftEditFlag.bind(draftUpdateCmd, false)
	draftEditFlag.bindAttach(draftUpdateCmd)
	draftUpdateCmd.Flags().Int64SliceVar(&flagDraftDrop, "remove-attachment", nil, "Attachment id to drop from the draft, repeatable")
	draftDeleteCmd.Flags().BoolVar(&flagDraftYes, "yes", false, "Answer the confirmation with yes")
	draftCmd.AddCommand(draftCreateCmd, draftUpdateCmd, draftDeleteCmd)
}

// draftReport is the --json document for a draft write.
type draftReport struct {
	AccountID int64 `json:"account_id"`
	ID        int64 `json:"id"`
	Deleted   bool  `json:"deleted,omitempty"`
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
	if err := checkAddresses(draftFlags.to, draftFlags.cc, draftFlags.bcc); err != nil {
		return err
	}
	if _, err := checkAttachments(draftFlags.attach, bodyText(text)+bodyText(html)); err != nil {
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

// runDraftUpdate merges the flags over the stored draft and saves the result
// in place, so the draft keeps its id.
func runDraftUpdate(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("%q is not a draft id — ids are the numbers in `courrier list drafts`", args[0])
	}
	text, html, err := draftEditFlag.bodies(cmd)
	if err != nil {
		return err
	}
	if _, err := checkAttachments(draftEditFlag.attach, bodyText(text)+bodyText(html)); err != nil {
		return err
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	stored, err := loadDraft(ctx, api, accountID, id)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	req := mergeDraft(cmd, &draftEditFlag, stored, text, html)
	req.Attachments = draftEditFlag.attach
	if err := checkAddresses(req.To, req.Cc, req.Bcc); err != nil {
		return err
	}
	if _, err := api.UpdateDraft(ctx, accountID, id, req, flagDraftDrop); err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot save the changed draft — %w", err))
	}
	return reportDraft(draftReport{AccountID: accountID, ID: id}, "draft %d updated", id)
}

// bodyText dereferences an optional body for a size count.
func bodyText(body *string) string {
	if body == nil {
		return ""
	}
	return *body
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
