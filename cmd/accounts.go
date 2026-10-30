package cmd

import (
	"fmt"

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

var accountsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check the account's IMAP and SMTP credentials",
	Long: `Sign in to IMAP and SMTP with the stored passwords and disconnect again.

Nothing is read or written. Both protocols are always tried and reported apart,
so "IMAP works, SMTP does not" is visible. A failed login exits 1.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return runAccountsCheck() },
}

// init registers the check subcommand.
func init() {
	accountsCmd.AddCommand(accountsCheckCmd)
}

// runAccountsCheck probes the resolved account and fails when a leg fails.
func runAccountsCheck() error {
	ctx, stop := signalContext()
	defer stop()

	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	result, err := api.CheckAccount(ctx, accountID)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if flagJSON {
		if err := ui.JSON(result); err != nil {
			return err
		}
	} else {
		reportLeg("imap", result.IMAP)
		reportLeg("smtp", result.SMTP)
	}
	if (result.IMAP.Configured && !result.IMAP.OK) || (result.SMTP.Configured && !result.SMTP.OK) {
		return fmt.Errorf("account %d failed its connection check", accountID)
	}
	return nil
}

// reportLeg prints one protocol's outcome.
func reportLeg(name string, leg client.CheckLeg) {
	switch {
	case !leg.Configured:
		ui.Plain("%s not configured", name)
	case leg.OK:
		ui.Success("%s ok", name)
	default:
		ui.Warn("%s failed — %s", name, leg.Error)
	}
}
