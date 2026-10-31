package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	flagFolderForce bool
	flagFolderYes   bool
)

var foldersCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a folder",
	Long: `Create a top-level folder on the IMAP server.

The name is 1 to 100 characters, without the server's hierarchy delimiter, / \
* % or a leading dot, and cannot read as a system folder (inbox, sent, drafts,
trash, junk, archive): a sync would file it under that type.`,
	Args: cobra.ExactArgs(1),
	RunE: runFolderCreate,
}

var foldersRenameCmd = &cobra.Command{
	Use:   "rename <folder> <new-name>",
	Short: "Rename a custom folder",
	Long: `Rename a custom folder. <folder> is a folder id or its name. System folders
cannot be renamed.`,
	Args: cobra.ExactArgs(2),
	RunE: runFolderRename,
}

var foldersDeleteCmd = &cobra.Command{
	Use:   "delete <folder>",
	Short: "Delete a custom folder",
	Long: `Delete a custom folder. <folder> is a folder id or its name. System folders
cannot be deleted.

IMAP deletes the messages in a folder with it, so a folder that still holds
messages is refused unless you pass --force. Move them out first with
courrier move. Deleting asks first on a terminal; pass --yes to skip the
question.`,
	Args: cobra.ExactArgs(1),
	RunE: runFolderDelete,
}

// init attaches the folder write commands to the folders listing.
func init() {
	foldersDeleteCmd.Flags().BoolVar(&flagFolderForce, "force", false, "Delete the folder even when it holds messages")
	foldersDeleteCmd.Flags().BoolVar(&flagFolderYes, "yes", false, "Answer the confirmation with yes")
	foldersCmd.AddCommand(foldersCreateCmd, foldersRenameCmd, foldersDeleteCmd)
}

// runFolderCreate creates the folder and prints it.
func runFolderCreate(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	folder, err := api.CreateFolder(ctx, accountID, args[0])
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot create the folder — %w", err))
	}
	return reportFolder(folder, "folder %q created as id %d", folder.Name, folder.ID)
}

// runFolderRename renames a custom folder.
func runFolderRename(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	folder, err := customFolder(ctx, api, accountID, args[0], "renamed")
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	renamed, err := api.RenameFolder(ctx, accountID, folder.ID, args[1])
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot rename the folder — %w", err))
	}
	return reportFolder(renamed, "folder %d renamed to %q", renamed.ID, renamed.Name)
}

// runFolderDelete deletes a custom folder after the usual confirmation.
func runFolderDelete(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	folder, err := customFolder(ctx, api, accountID, args[0], "deleted")
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if err := confirmDestructive(fmt.Sprintf("folder %q and any messages in it", folder.Name), flagFolderYes); err != nil {
		return err
	}
	if err := api.DeleteFolder(ctx, accountID, folder.ID, flagFolderForce); err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot delete the folder — %w", err))
	}
	if flagJSON {
		return ui.JSON(map[string]any{"account_id": accountID, "id": folder.ID, "deleted": true})
	}
	ui.Success("folder %q deleted", folder.Name)
	return nil
}

// customFolder resolves a folder reference and refuses a system folder before
// any write.
func customFolder(ctx context.Context, api *client.Client, accountID int64, ref, verb string) (client.Folder, error) {
	folders, err := api.Folders(ctx, accountID)
	if err != nil {
		return client.Folder{}, err
	}
	folder, err := findFolder(folders, ref)
	if err != nil {
		return folder, err
	}
	if folder.Type != "custom" {
		return folder, fmt.Errorf("%q is a system folder (%s) and cannot be %s", folder.Name, folder.Type, verb)
	}
	return folder, nil
}

// reportFolder prints a folder as JSON or one success line.
func reportFolder(folder client.Folder, format string, a ...any) error {
	if flagJSON {
		return ui.JSON(folder)
	}
	ui.Success(format, a...)
	return nil
}
