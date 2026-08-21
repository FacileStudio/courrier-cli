package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// folderTypes is the set the listing route addresses folders by.
//
// This is the API's one asymmetry: a listing takes a folder TYPE, a sync takes a
// folder ID. A folder id typed here reaches a route that has no such type and
// answers a bare 404, which reads as "no such mailbox" rather than "wrong kind
// of identifier" — so the argument is checked against this list before a request
// is spent on it.
var folderTypes = []string{"inbox", "sent", "drafts", "trash", "junk", "archive", "custom"}

// maxListLimit is the server's page ceiling. It reduces anything larger without
// saying so, so a caller who asked for 500 and got 100 would have no way to tell
// the folder ran out from the cap being hit.
const maxListLimit = 100

var (
	flagListUnread bool
	flagListLimit  int
	flagListPage   int
)

// inboxCmd is the shorthand a mail client is expected to have: the inbox
// without having to name it.
var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "List conversations in the inbox",
	Long: `List the inbox as collapsed conversations, newest first.

A row is a conversation, not a message. message_count says how many messages it
holds and email_ids names every one of them, so a row with a count of 4 is four
messages under one subject. Expand one with courrier read <thread-id>.

Equivalent to courrier list inbox.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList("inbox")
	},
}

// listCmd lists any folder, addressed by type.
var listCmd = &cobra.Command{
	Use:   "list <folder-type>",
	Short: "List conversations in a folder",
	Long: `List one folder as collapsed conversations, newest first.

The argument is a folder TYPE — inbox, sent, drafts, trash, junk, archive or
custom — and never a folder id. courrier folders shows the type of each folder
next to its id.

A row is a conversation, not a message. message_count says how many messages it
holds and email_ids names every one of them, so a row with a count of 4 is four
messages under one subject. Expand one with courrier read <thread-id>.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(args[0])
	},
}

// init registers the paging flags on both spellings of the same command. They
// share one set of variables because only one of the two ever runs.
func init() {
	for _, command := range []*cobra.Command{inboxCmd, listCmd} {
		command.Flags().BoolVar(&flagListUnread, "unread", false, "Show only conversations holding unread mail")
		command.Flags().IntVar(&flagListLimit, "limit", 50, "Conversations per page, at most 100")
		command.Flags().IntVar(&flagListPage, "page", 1, "Page to show, counting from 1")
	}
}

// runList prints one page of a folder, and is the whole of both inbox and list.
func runList(folderType string) error {
	folderType = strings.ToLower(strings.TrimSpace(folderType))
	if !slices.Contains(folderTypes, folderType) {
		return fmt.Errorf("unknown folder type %q — use one of %s", folderType, strings.Join(folderTypes, ", "))
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

	page, err := api.ListEmails(ctx, accountID, folderType, client.ListOptions{
		Page:       flagListPage,
		Limit:      clampLimit(flagListLimit),
		UnreadOnly: flagListUnread,
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
}

// clampLimit holds a requested page size under the server's ceiling and says so
// on stderr, rather than sending a number the instance will quietly reduce.
func clampLimit(limit int) int {
	if limit > maxListLimit {
		ui.Warn("--limit %d is above the instance ceiling — asking for %d", limit, maxListLimit)
		return maxListLimit
	}
	return limit
}

// wrapInterrupt maps a cancelled context onto ErrInterrupted, so a read stopped
// with Ctrl-C exits 130 instead of reporting the aborted request as a failure of
// the instance.
func wrapInterrupt(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ErrInterrupted
	}
	return err
}
