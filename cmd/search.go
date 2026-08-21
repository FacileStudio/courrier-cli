package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	flagSearchLimit int
	flagSearchPage  int
)

// searchCmd matches a query against subject, sender and plain-text body.
//
// The arguments are joined with a space rather than taken as one, so an
// unquoted multi-word query behaves the way a shell user expects instead of
// erroring on the second word.
//
// A blank query is not an error here. The instance answers it with an empty
// page, so it renders as an empty result and exits 0 — inventing a client-side
// failure would disagree with the API for no gain.
//
// Rows are messages, not the collapsed conversations a folder listing returns:
// a search matches individual mail, and grouping it by thread would hide which
// message actually matched.
var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search subject, sender and body",
	Long: `Search the account's mail by subject, sender and plain-text body.

Trash and junk are skipped. Every argument is joined with a space, so quoting a
multi-word query is optional:

  courrier search invoice march
  courrier search "invoice march"

Rows are the matching messages themselves, not collapsed conversations. Read the
conversation one belongs to with courrier read <thread-id>, or the message alone
with courrier read --id <email-id>.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		page, err := api.Search(ctx, accountID, strings.Join(args, " "), client.ListOptions{
			Page:  flagSearchPage,
			Limit: clampLimit(flagSearchLimit),
		})
		if err != nil {
			return wrapInterrupt(ctx, err)
		}
		if page.Emails == nil {
			page.Emails = []client.Email{}
		}

		if flagJSON {
			return ui.JSON(page)
		}

		renderEmailRows(page.Emails)
		renderPageSummary(page)
		return nil
	},
}

// init registers the paging flags. The default page is smaller than a folder
// listing's because a search is scanned, not browsed.
func init() {
	searchCmd.Flags().IntVar(&flagSearchLimit, "limit", 30, "Results per page, at most 100")
	searchCmd.Flags().IntVar(&flagSearchPage, "page", 1, "Page to show, counting from 1")
}
