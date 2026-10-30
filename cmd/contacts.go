package cmd

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	flagContactsAll   bool
	flagContactsLimit int
	flagContactsPage  int
	flagContactsQuery string
)

// contactsCmd lists the addresses the user has written to, which is what the
// server counts as known senders.
var contactsCmd = &cobra.Command{
	Use:   "contacts",
	Short: "List the addresses you have written to",
	Long: `List the addresses you have written to, most recent first.

These are the people triage calls known. With no --account and no default
account, every account is covered; with a default set, pass --all.`,
	Args:    cobra.NoArgs,
	PreRunE: usage(checkContactsFlags),
	RunE:    func(cmd *cobra.Command, args []string) error { return runContacts() },
}

// init registers the contacts flags.
func init() {
	flags := contactsCmd.Flags()
	flags.BoolVar(&flagContactsAll, "all", false, "Cover every account, even when a default is set")
	flags.IntVar(&flagContactsLimit, "limit", 50, "Contacts per page, at most 100")
	flags.StringVar(&flagContactsQuery, "query", "", "Autocomplete addresses on one account by name or email")
	flags.IntVar(&flagContactsPage, "page", 1, "Page to show, counting from 1")
}

// runContacts fetches one page of contacts and prints it.
func runContacts() error {
	ctx, stop := signalContext()
	defer stop()

	api, cfg, err := session()
	if err != nil {
		return err
	}
	if flagContactsQuery != "" {
		return runContactsSearch(ctx, api, cfg)
	}
	account, err := scopeAccount(cfg, flagContactsAll)
	if err != nil {
		return err
	}
	out, err := api.Contacts(ctx, client.ContactsQuery{
		Limit:   clampLimit(flagContactsLimit),
		Page:    flagContactsPage,
		Account: account,
	})
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if flagJSON {
		return ui.JSON(out.Raw)
	}
	renderContacts(out.Contacts)
	return nil
}

// renderContacts prints one line per contact.
func renderContacts(contacts []client.Contact) {
	if len(contacts) == 0 {
		ui.Plain("No contacts")
		return
	}
	rows := make([][]string, 0, len(contacts))
	for _, contact := range contacts {
		rows = append(rows, []string{
			relativeTime(contact.LastSentAt),
			strconv.Itoa(contact.SentCount),
			truncate(contact.Name, 28),
			contact.Email,
		})
	}
	ui.Table([]string{"LAST SENT", "SENT", "NAME", "EMAIL"}, rows)
}

// runContactsSearch autocompletes addresses on the resolved account.
func runContactsSearch(ctx context.Context, api *client.Client, cfg config.Config) error {
	accountID, err := resolveAccount(ctx, api, cfg)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	matches, err := api.SearchContacts(ctx, accountID, flagContactsQuery)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if matches == nil {
		matches = []client.ContactMatch{}
	}
	if flagJSON {
		return ui.JSON(matches)
	}
	if len(matches) == 0 {
		ui.Plain("No contacts")
		return nil
	}
	rows := make([][]string, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, []string{strconv.Itoa(match.Count), truncate(match.Name, 28), match.Email})
	}
	ui.Table([]string{"USES", "NAME", "EMAIL"}, rows)
	return nil
}
