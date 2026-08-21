package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var flagReadID int64

// readCmd prints every message in one conversation, oldest first.
//
// The positional argument is a thread id, which Courrier keys by Message-ID
// rather than by a number of its own. It is passed through exactly as typed —
// the client percent-encodes it into the path, so nothing here needs to escape
// it, and mangling it first would only produce a 404.
//
// --id is the other way in and reads a single message by its numeric id. It
// renders through the same thread view over a one-message slice, so the output
// shape and the --json document are identical either way and a parser never has
// to branch on which spelling was used.
var readCmd = &cobra.Command{
	Use:   "read [thread-id]",
	Short: "Show every message in a conversation",
	Long: `Show every message in one conversation, oldest first, as plain text.

The thread id is a Message-ID, like <CAJ8f=abc@mail.example.com>. It carries <,
> and usually =, all of which a shell will try to interpret, so quote it:

  courrier read '<CAJ8f=abc@mail.example.com>'

Thread ids come from a listing:

  courrier inbox --json | jq -r '.emails[].thread_id'

To read one message rather than its whole conversation, pass --id with the
numeric email id from the ID column instead of a thread id.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		threadID := ""
		if len(args) == 1 {
			threadID = args[0]
		}
		switch {
		case threadID == "" && flagReadID == 0:
			return errors.New("read needs something to show — pass a thread id, or --id <email-id> for a single message")
		case threadID != "" && flagReadID != 0:
			return errors.New("read takes a thread id or --id, not both — drop one")
		}

		ctx, stop := signalContext()
		defer stop()

		api, cfg, err := session()
		if err != nil {
			return err
		}
		accountID, err := resolveAccount(ctx, api, cfg)
		if err != nil {
			return wrapInterrupt(ctx, err)
		}

		var emails []client.Email
		if flagReadID != 0 {
			email, err := api.Email(ctx, accountID, flagReadID)
			if err != nil {
				return wrapInterrupt(ctx, err)
			}
			emails = []client.Email{email}
		} else {
			emails, err = api.Thread(ctx, accountID, threadID)
			if err != nil {
				return wrapInterrupt(ctx, err)
			}
		}
		if emails == nil {
			emails = []client.Email{}
		}

		if flagJSON {
			return ui.JSON(emails)
		}

		if len(emails) == 0 {
			ui.Plain("No mail")
			return nil
		}
		renderThread(emails)
		return nil
	},
}

// init registers the single-message alternative to a thread id.
func init() {
	readCmd.Flags().Int64Var(&flagReadID, "id", 0, "Read one message by its numeric email id instead of a conversation")
}
