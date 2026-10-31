package cmd

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// deliver checks, confirms and sends a composed message, then reports it the
// way send does so a caller parses one shape for every way of sending.
func deliver(ctx context.Context, api *client.Client, accountID int64, f *composeFlags, req client.SendRequest) error {
	files, err := checkAttachments(req.Attachments, req.Body+req.BodyHTML)
	if err != nil {
		return err
	}
	if err := confirmDelivery(f, req, files); err != nil {
		return err
	}

	progress("sending to %s", strings.Join(slices.Concat(req.To, req.Bcc), ", "))
	if err := api.Send(ctx, accountID, req); err != nil {
		return sendFailure(ctx, err)
	}
	if flagJSON {
		return ui.JSON(sendReport{
			AccountID:   accountID,
			Sent:        true,
			To:          req.To,
			Cc:          append([]string{}, req.Cc...),
			Bcc:         append([]string{}, req.Bcc...),
			Subject:     req.Subject,
			Attachments: files,
		})
	}
	ui.Success("sent to %s", strings.Join(slices.Concat(req.To, req.Bcc), ", "))
	return nil
}

// confirmDelivery shows what is about to leave and asks, under the same rules
// as send: --yes skips the question and is required wherever nothing can answer.
func confirmDelivery(f *composeFlags, req client.SendRequest, files []attachment) error {
	if f.yes {
		return nil
	}
	if flagJSON {
		return errors.New("--json needs --yes to send — structured output never prompts")
	}
	if f.usedStdin || !stdinIsTerminal() {
		return errors.New("sending needs --yes when a body comes from stdin or stdin is not a terminal")
	}

	ui.Warn("about to send this message")
	printRecipients(req.To, req.Cc, req.Bcc)
	ui.Plain("subject %s", req.Subject)
	for _, file := range files {
		ui.Plain("file    %s (%s)", file.Path, humanSize(file.Size))
	}
	ui.Plain("")
	ui.Plain("%s", previewBody(req.Body))
	ui.Plain("")

	confirmed, err := confirmAction("send it?")
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("send cancelled — nothing was sent")
	}
	return nil
}

// confirmDestructive asks before a deletion that nothing undoes, with the same
// gate as mark --delete.
func confirmDestructive(what string, yes bool) error {
	if yes {
		return nil
	}
	if flagJSON {
		return errors.New("deleting with --json needs --yes — structured output never prompts")
	}
	if !stdinIsTerminal() {
		return errors.New("deleting needs --yes when stdin is not a terminal — nothing there can answer the confirmation")
	}
	ui.Warn("about to permanently delete %s", what)
	confirmed, err := confirmAction("delete it?")
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("delete cancelled — nothing was changed")
	}
	return nil
}

// printRecipients shows the address lines of a confirmation, leaving out the
// empty ones.
func printRecipients(to, cc, bcc []string) {
	ui.Plain("to      %s", strings.Join(to, ", "))
	if len(cc) > 0 {
		ui.Plain("cc      %s", strings.Join(cc, ", "))
	}
	if len(bcc) > 0 {
		ui.Plain("bcc     %s", strings.Join(bcc, ", "))
	}
}
