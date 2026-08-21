package cmd

import (
	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// accountsCmd lists the mail accounts the session can act on, marking the one
// every other command uses by default.
//
// The resolution error is deliberately dropped: with several accounts and no
// stored default, resolveAccount refuses to guess — and the list this command
// prints is exactly the answer to that refusal, so failing here would withhold
// the very thing the user came for. An unmarked list is the honest rendering of
// "nothing is the default yet".
var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "List mail accounts",
	Long: `List the IMAP mailboxes configured on this instance.

The account other commands act on by default is marked. Override it for one
command with --account <id>, or for a whole shell with COURRIER_ACCOUNT.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signalContext()
		defer stop()

		api, cfg, err := session()
		if err != nil {
			return err
		}

		accounts, err := api.Accounts(ctx)
		if err != nil {
			return wrapInterrupt(ctx, err)
		}
		if accounts == nil {
			accounts = []client.Account{}
		}

		if flagJSON {
			return ui.JSON(accounts)
		}

		current, _ := resolveAccount(ctx, api, cfg)
		renderAccounts(accounts, current)
		return nil
	},
}
