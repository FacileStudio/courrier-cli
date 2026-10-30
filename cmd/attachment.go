package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	flagAttachmentOut   string
	flagAttachmentForce bool
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment <email-id> <attachment-id>",
	Short: "Download an attachment",
	Long: `Download one attachment of a message.

Attachment ids come from courrier read --id <email-id> --json, in the
attachments array. The file lands in the current directory under its own name;
--output picks another path, or - writes the bytes to stdout. An existing file
is never overwritten without --force.`,
	Args: cobra.ExactArgs(2),
	RunE: runAttachment,
}

// init registers the attachment flags.
func init() {
	attachmentCmd.Flags().StringVarP(&flagAttachmentOut, "output", "o", "", "Write to this path, or - for stdout")
	attachmentCmd.Flags().BoolVar(&flagAttachmentForce, "force", false, "Overwrite an existing file")
}

// attachmentReport is the --json document for a download.
type attachmentReport struct {
	AccountID    int64  `json:"account_id"`
	EmailID      int64  `json:"email_id"`
	AttachmentID int64  `json:"attachment_id"`
	Path         string `json:"path"`
	Size         int    `json:"size"`
}

// runAttachment downloads one attachment to disk or stdout.
func runAttachment(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	emailID, err := parseEmailID(args[0])
	if err != nil {
		return err
	}
	attachmentID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || attachmentID <= 0 {
		return fmt.Errorf("%q is not an attachment id — see the attachments array of `courrier read --id %d --json`", args[1], emailID)
	}
	if flagAttachmentOut == "-" && flagJSON {
		return errors.New("--output - and --json both want stdout — write to a file with --output <path>")
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	email, err := api.Email(ctx, accountID, emailID)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot read email %d — %w", emailID, err))
	}
	raw, err := api.DownloadAttachment(ctx, accountID, emailID, attachmentID)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot download attachment %d — %w", attachmentID, err))
	}
	path, err := writeAttachment(raw, attachmentName(email, attachmentID))
	if err != nil {
		return err
	}
	return reportAttachment(attachmentReport{accountID, emailID, attachmentID, path, len(raw)})
}

// attachmentName finds the filename the server recorded for an attachment.
func attachmentName(email client.Email, id int64) string {
	for _, attachment := range email.Attachments {
		if attachment.ID == id {
			return safeName(attachment.Filename)
		}
	}
	return "attachment-" + strconv.FormatInt(id, 10)
}

// writeAttachment saves the bytes where --output says, refusing to clobber.
func writeAttachment(raw []byte, name string) (string, error) {
	path := flagAttachmentOut
	if path == "-" {
		_, err := ui.Stdout().Write(raw)
		return "-", err
	}
	if path == "" {
		path = name
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if flagAttachmentForce {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return "", fmt.Errorf("cannot write %s — %w (pass --force to overwrite)", path, err)
	}
	defer file.Close()
	if _, err := file.Write(raw); err != nil {
		return "", fmt.Errorf("cannot write %s — %w", path, err)
	}
	return path, nil
}

// reportAttachment prints the outcome, staying silent when the bytes went to
// stdout.
func reportAttachment(report attachmentReport) error {
	switch {
	case flagJSON:
		return ui.JSON(report)
	case report.Path != "-":
		ui.Success("saved %s (%s)", report.Path, humanSize(int64(report.Size)))
	}
	return nil
}
