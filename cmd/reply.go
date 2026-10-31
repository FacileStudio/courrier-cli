package cmd

import (
	"context"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

var (
	replyFlags    composeFlags
	forwardFlags  composeFlags
	flagReplyAll  bool
	flagForwardNo bool
)

var replyCmd = &cobra.Command{
	Use:   "reply <email-id>",
	Short: "Reply to a message",
	Long: `Reply to one message with the threading headers set.

The recipient is the original sender, the subject gains "Re: ", In-Reply-To and
References point at the original, and the original text is quoted under your
body. --all also copies the other To and Cc recipients, minus the account's own
address. --to and --cc replace the computed recipients when given.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runReply(cmd, args[0]) },
}

var forwardCmd = &cobra.Command{
	Use:   "forward <email-id>",
	Short: "Forward a message",
	Long: `Forward one message to new recipients.

The subject gains "Fwd: ", the original headers and text follow your optional
note, and the original attachments are downloaded and re-sent unless
--no-attachments is given.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runForward(cmd, args[0]) },
}

// init wires the reply and forward flags.
func init() {
	replyFlags.bind(replyCmd, true)
	replyCmd.Flags().BoolVar(&flagReplyAll, "all", false, "Reply to every recipient, not only the sender")
	forwardFlags.bind(forwardCmd, true)
	forwardCmd.Flags().BoolVar(&flagForwardNo, "no-attachments", false, "Do not re-send the original attachments")
	_ = forwardCmd.MarkFlagRequired("to")
}

// runReply fetches the original, builds the reply and delivers it.
func runReply(cmd *cobra.Command, rawID string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := parseEmailID(rawID)
	if err != nil {
		return err
	}
	text, htmlBody, err := replyFlags.bodies(cmd)
	if err != nil {
		return err
	}
	if text, err = replyFlags.stdinBody(text, htmlBody); err != nil {
		return err
	}
	if text == nil && htmlBody == nil {
		return fmt.Errorf("no reply body — pass --body, --body-file, --html, --html-file or pipe text into stdin")
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	original, err := api.Email(ctx, accountID, id)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot read email %d — %w", id, err))
	}
	self := accountAddress(ctx, api, accountID)
	req := buildReply(replyFlags, original, text, htmlBody, flagReplyAll, self, cmd)
	if err := checkAddresses(req.To, req.Cc, req.Bcc); err != nil {
		return err
	}
	return deliver(ctx, api, accountID, &replyFlags, req)
}

// runForward fetches the original and its attachments and delivers the copy.
func runForward(cmd *cobra.Command, rawID string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := parseEmailID(rawID)
	if err != nil {
		return err
	}
	text, htmlBody, err := forwardFlags.bodies(cmd)
	if err != nil {
		return err
	}
	if err := checkAddresses(forwardFlags.to, forwardFlags.cc, forwardFlags.bcc); err != nil {
		return err
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	original, err := api.Email(ctx, accountID, id)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot read email %d — %w", id, err))
	}
	req := buildForward(forwardFlags, original, text, htmlBody)
	if !flagForwardNo {
		dir, files, err := fetchAttachments(ctx, api, accountID, original)
		if err != nil {
			return wrapInterrupt(ctx, err)
		}
		defer os.RemoveAll(dir)
		req.Attachments = append(req.Attachments, files...)
	}
	return deliver(ctx, api, accountID, &forwardFlags, req)
}

// parseEmailID parses one numeric email id.
func parseEmailID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not an email id — ids are the numbers in the ID column of `courrier inbox`", raw)
	}
	return id, nil
}

// accountAddress returns the account's own address, or empty when it cannot be
// found, in which case --all keeps every recipient.
func accountAddress(ctx context.Context, api *client.Client, accountID int64) string {
	accounts, err := api.Accounts(ctx)
	if err != nil {
		return ""
	}
	for _, account := range accounts {
		if account.ID == accountID {
			return account.Email
		}
	}
	return ""
}

// fetchAttachments downloads each attachment into its own subdirectory of a
// temp dir, so equal filenames never collide, and returns the paths.
func fetchAttachments(ctx context.Context, api *client.Client, accountID int64, original client.Email) (string, []string, error) {
	dir, err := os.MkdirTemp("", "courrier-forward-")
	if err != nil {
		return "", nil, err
	}
	paths := make([]string, 0, len(original.Attachments))
	for i, attachment := range original.Attachments {
		raw, err := api.DownloadAttachment(ctx, accountID, original.ID, attachment.ID)
		if err != nil {
			os.RemoveAll(dir)
			return "", nil, fmt.Errorf("cannot download attachment %s — %w", attachment.Filename, err)
		}
		sub := fmt.Sprintf("%s/%d", dir, i)
		path := sub + "/" + safeName(attachment.Filename)
		if err := os.Mkdir(sub, 0o700); err != nil {
			os.RemoveAll(dir)
			return "", nil, err
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			os.RemoveAll(dir)
			return "", nil, err
		}
		paths = append(paths, path)
	}
	return dir, paths, nil
}

// safeName keeps only the final element of a server-supplied filename.
func safeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

// escapeHTML wraps plain text as HTML for quoting.
func escapeHTML(text string) string {
	return strings.ReplaceAll(html.EscapeString(text), "\n", "<br>\n")
}
