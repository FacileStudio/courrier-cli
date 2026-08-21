package cmd

import (
	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// foldersCmd lists the account's mailboxes with the counts the instance keeps.
//
// It is also the lookup table for the two ways Courrier addresses a folder: the
// TYPE column is what a listing takes, the ID column is what a per-folder sync
// takes, and confusing the two is the API's one real trap.
var foldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "List folders with their counts",
	Long: `List the account's IMAP folders with their unread and total counts.

The TYPE column is what a listing takes: courrier list <folder-type>. The ID
column is what a single-folder sync takes: courrier sync --folder <id>.`,
	Args: cobra.NoArgs,
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

		folders, err := api.Folders(ctx, accountID)
		if err != nil {
			return wrapInterrupt(ctx, err)
		}
		if folders == nil {
			folders = []client.Folder{}
		}

		if flagJSON {
			return ui.JSON(folders)
		}

		renderFolders(folders)
		return nil
	},
}
