package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

const (
	// sendPerMinute is the instance's limit, named in the message a 429
	// produces so the reader knows what to wait for.
	sendPerMinute = 10

	// maxMessageBytes is where the instance stops parsing the multipart body and
	// answers 413. The body counts against it as well as the attachments, which
	// is why the check totals both.
	maxMessageBytes = 25 << 20
)

var (
	flagSendTo        []string
	flagSendCc        []string
	flagSendSubject   string
	flagSendBody      string
	flagSendBodyFile  string
	flagSendAttach    []string
	flagSendInReplyTo string
	flagSendYes       bool
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message",
	Long: `Send a message from the account over its SMTP server.

The body comes from --body, from --body-file (- reads stdin), or from stdin
itself when neither flag is given and something is piped in. There is no editor:
a terminal with no body flag and nothing piped in is an error, because an editor
cannot be scripted and an agent cannot close one.

Attachments are checked before anything is sent — every path is opened, and the
total is measured against the 25 MB the instance parses — since a send that
fails halfway has already delivered and cannot be taken back.

Sending cannot be undone, so it asks first on a terminal. Pass --yes to skip the
question; --yes is required when stdin is not a terminal or has been spent
reading the body.`,
	Args: cobra.NoArgs,
	RunE: runSend,
}

// init wires the send flags.
//
// --to and --cc are slices, which split on commas, so one flag can carry a list
// of addresses. --attach is an array, which does not split: a comma is legal in
// a filename and silently halving a path is worse than making the caller repeat
// the flag.
func init() {
	sendCmd.Flags().StringSliceVar(&flagSendTo, "to", nil, "Recipient address, repeatable")
	sendCmd.Flags().StringSliceVar(&flagSendCc, "cc", nil, "Carbon copy address, repeatable")
	sendCmd.Flags().StringVar(&flagSendSubject, "subject", "", "Subject line")
	sendCmd.Flags().StringVar(&flagSendBody, "body", "", "Message body as plain text")
	sendCmd.Flags().StringVar(&flagSendBodyFile, "body-file", "", "Read the message body from this file, or - for stdin")
	sendCmd.Flags().StringArrayVar(&flagSendAttach, "attach", nil, "Attach this local file, repeatable")
	sendCmd.Flags().StringVar(&flagSendInReplyTo, "in-reply-to", "", "Message-ID this message replies to")
	sendCmd.Flags().BoolVar(&flagSendYes, "yes", false, "Answer the confirmation with yes")

	_ = sendCmd.MarkFlagRequired("to")
}

// attachment is one validated file: the path to send it from, and the size that
// counts against the instance's multipart cap.
type attachment struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// sendReport is the --json document for a send.
type sendReport struct {
	AccountID   int64        `json:"account_id"`
	Sent        bool         `json:"sent"`
	To          []string     `json:"to"`
	Cc          []string     `json:"cc"`
	Subject     string       `json:"subject"`
	Attachments []attachment `json:"attachments"`
}

// runSend assembles the message and checks all of it before the session is even
// opened, so a bad address or an unreadable attachment costs nothing.
func runSend(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	body, fromStdin, err := resolveBody(cmd)
	if err != nil {
		return err
	}
	if err := validateRecipients(); err != nil {
		return err
	}
	attachments, err := checkAttachments(flagSendAttach, body)
	if err != nil {
		return err
	}

	api, cfg, err := session()
	if err != nil {
		return err
	}
	accountID, err := resolveAccount(ctx, api, cfg)
	if err != nil {
		return err
	}

	if err := confirmSend(body, attachments, fromStdin); err != nil {
		return err
	}

	progress("sending to %s", strings.Join(flagSendTo, ", "))
	err = api.Send(ctx, accountID, client.SendRequest{
		To:          flagSendTo,
		Cc:          flagSendCc,
		Subject:     flagSendSubject,
		Body:        body,
		InReplyTo:   flagSendInReplyTo,
		Attachments: flagSendAttach,
	})
	if err != nil {
		return sendFailure(ctx, err)
	}

	if flagJSON {
		return ui.JSON(sendReport{
			AccountID:   accountID,
			Sent:        true,
			To:          flagSendTo,
			Cc:          append([]string{}, flagSendCc...),
			Subject:     flagSendSubject,
			Attachments: attachments,
		})
	}
	ui.Success("sent to %s", strings.Join(flagSendTo, ", "))
	return nil
}

// resolveBody decides where the message text comes from and reports whether it
// came from stdin, which matters later: stdin read to EOF cannot then answer a
// confirmation.
//
// The flags are read through Changed rather than compared to "", so a
// deliberately empty --body sends an empty message instead of falling through to
// stdin and blocking on a terminal nobody is typing into.
func resolveBody(cmd *cobra.Command) (string, bool, error) {
	hasBody := cmd.Flags().Changed("body")
	hasFile := cmd.Flags().Changed("body-file")

	switch {
	case hasBody && hasFile:
		return "", false, errors.New("--body and --body-file cannot be combined — pass one or the other")
	case hasBody:
		return flagSendBody, false, nil
	case hasFile && flagSendBodyFile == "-":
		return readStdinBody()
	case hasFile:
		raw, err := os.ReadFile(flagSendBodyFile)
		if err != nil {
			return "", false, fmt.Errorf("cannot read the body file — %w", err)
		}
		return string(raw), false, nil
	case !stdinIsTerminal():
		return readStdinBody()
	}
	return "", false, errors.New("no message body — pass --body, --body-file <path>, or pipe the text into stdin")
}

// readStdinBody reads all of stdin as the message body.
func readStdinBody() (string, bool, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", true, fmt.Errorf("cannot read the message body from stdin — %w", err)
	}
	return string(raw), true, nil
}

// validateRecipients refuses locally what the SMTP server would refuse anyway,
// which is cheaper and clearer than a relay's rejection prose.
func validateRecipients() error {
	if len(flagSendTo) == 0 {
		return errors.New("no recipient — pass --to <address>")
	}
	for _, address := range slices.Concat(flagSendTo, flagSendCc) {
		if !strings.Contains(address, "@") {
			return fmt.Errorf("%q is not an email address — check the --to and --cc values", address)
		}
	}
	return nil
}

// checkAttachments opens every file before anything is sent.
//
// Half a send is not recoverable: the message is either on its way to every
// recipient or to none, and finding out that the third attachment is a directory
// after the first two uploaded helps nobody. The total includes the body and is
// measured against the instance's parse cap, so an oversized message is refused
// here in a moment rather than after a minute of upload and a 413.
func checkAttachments(paths []string, body string) ([]attachment, error) {
	checked := make([]attachment, 0, len(paths))
	total := int64(len(body))

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read the attachment %s — %w", path, err)
		}
		info, err := file.Stat()
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot read the attachment %s — %w", path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s is a directory — attach files, not folders", path)
		}

		checked = append(checked, attachment{Path: path, Size: info.Size()})
		total += info.Size()
	}

	if total > maxMessageBytes {
		return nil, fmt.Errorf("the message and its attachments come to %s, over the %s the instance accepts — send fewer or smaller files",
			humanSize(total), humanSize(maxMessageBytes))
	}
	return checked, nil
}

// confirmSend shows what is about to leave and asks.
//
// Mail is the one thing here that no later command can take back, so the
// question is the default and --yes is the way past it. Structured output never
// prompts, and a stdin that is a pipe — or that has already been spent reading
// the body — has nothing left to answer with, so both demand --yes rather than
// hang on a question nobody will ever see.
func confirmSend(body string, attachments []attachment, bodyFromStdin bool) error {
	if flagSendYes {
		return nil
	}
	if flagJSON {
		return errors.New("--json needs --yes to send — structured output never prompts")
	}
	if bodyFromStdin || !stdinIsTerminal() {
		return errors.New("sending needs --yes when the body comes from stdin or stdin is not a terminal — nothing there can answer the confirmation")
	}

	ui.Warn("about to send this message")
	ui.Plain("to      %s", strings.Join(flagSendTo, ", "))
	if len(flagSendCc) > 0 {
		ui.Plain("cc      %s", strings.Join(flagSendCc, ", "))
	}
	ui.Plain("subject %s", subjectOrNone())
	for _, file := range attachments {
		ui.Plain("file    %s (%s)", file.Path, humanSize(file.Size))
	}
	ui.Plain("")
	ui.Plain("%s", previewBody(body))
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

// sendFailure explains the refusals a sender can act on.
//
// An interrupt is the awkward one: the request was already in flight, so the
// message may have gone out even though the command reports nothing. Saying so
// is the difference between one message and two.
func sendFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		ui.Warn("interrupted mid-send — the message may still have gone out, check the sent folder before retrying")
		return ErrInterrupted
	}

	switch {
	case rateLimited(err):
		return fmt.Errorf("the instance accepts %d messages a minute and refused this one — wait a minute and send it again", sendPerMinute)
	case tooLarge(err):
		return fmt.Errorf("the instance refused the message as too large — its multipart body caps at %s", humanSize(maxMessageBytes))
	}
	return fmt.Errorf("cannot send the message — %w", err)
}

// tooLarge reports whether the instance refused the message for its size, which
// the local check aims to prevent but cannot guarantee: multipart framing adds
// bytes this side does not count.
func tooLarge(err error) bool {
	var apiErr *client.Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusRequestEntityTooLarge
}

// subjectOrNone labels an empty subject in the confirmation, since a blank line
// there reads as a rendering bug rather than a decision.
func subjectOrNone() string {
	if strings.TrimSpace(flagSendSubject) == "" {
		return "(no subject)"
	}
	return flagSendSubject
}

// previewBody is the opening of the message, enough to catch the wrong draft
// without replaying a long one at the reader.
func previewBody(body string) string {
	const shown = 8

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) <= shown {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:shown], "\n") + ui.Dim(fmt.Sprintf("\n… %d more lines", len(lines)-shown))
}

// humanSize renders a byte count at a unit the reader can judge, so a small
// attachment does not display as "0.0 MB".
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", bytes)
}
