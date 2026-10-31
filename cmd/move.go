package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var flagMoveTo string

var moveCmd = &cobra.Command{
	Use:   "move <id>...",
	Short: "Move messages to a folder",
	Long: `Move one or more messages to a folder, at most 200 ids.

--to takes a system folder type (inbox, sent, drafts, trash, junk, archive), a
numeric folder id from courrier folders, or the name of a custom folder. The
messages keep their ids. The move happens on the IMAP server first, so a
refusal there leaves everything where it was.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMove,
}

// init wires the move flags.
func init() {
	moveCmd.Flags().StringVar(&flagMoveTo, "to", "", "Destination folder: a type, a folder id or a custom folder name")
}

// moveReport is the --json document for a move.
type moveReport struct {
	AccountID   int64   `json:"account_id"`
	Moved       []int64 `json:"moved"`
	Destination string  `json:"destination"`
}

// systemFolderTypes are the folder types a move can name without a lookup.
var systemFolderTypes = []string{"inbox", "sent", "drafts", "trash", "junk", "archive"}

// runMove validates the ids and the destination before any request, then moves
// the batch in one call.
func runMove(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	ids, err := parseEmailIDs(args)
	if err != nil {
		return err
	}
	if flagMoveTo == "" {
		commandStarted = false
		return errors.New("move needs a destination — pass --to <type, folder id or folder name>")
	}
	if strings.EqualFold(flagMoveTo, "custom") {
		return errors.New("custom names many folders — pass --to <folder id> or the folder's name")
	}
	api, accountID, err := openAccount(ctx)
	if err != nil {
		return err
	}
	target, err := moveTarget(ctx, api, accountID, flagMoveTo)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if err := api.MoveEmails(ctx, accountID, ids, target); err != nil {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		return fmt.Errorf("%s could not be moved to %s — %w", messageCount(len(ids)), flagMoveTo, err)
	}
	if flagJSON {
		return ui.JSON(moveReport{AccountID: accountID, Moved: ids, Destination: flagMoveTo})
	}
	ui.Success("%s moved to %s", messageCount(len(ids)), flagMoveTo)
	return nil
}

// moveTarget turns the --to value into what the API takes. A type and an id
// need no lookup; a name is matched against the account's folders.
func moveTarget(ctx context.Context, api *client.Client, accountID int64, ref string) (client.MoveTarget, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return client.MoveTarget{FolderID: id}, nil
	}
	lower := strings.ToLower(ref)
	for _, kind := range systemFolderTypes {
		if lower == kind {
			return client.MoveTarget{FolderType: kind}, nil
		}
	}
	folders, err := api.Folders(ctx, accountID)
	if err != nil {
		return client.MoveTarget{}, err
	}
	folder, err := findFolder(folders, ref)
	if err != nil {
		return client.MoveTarget{}, err
	}
	return client.MoveTarget{FolderID: folder.ID}, nil
}

// findFolder matches a folder by its id, or by name or path ignoring case. A
// name shared by several folders is refused rather than guessed.
func findFolder(folders []client.Folder, ref string) (client.Folder, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		folder, ok := folderByID(folders, id)
		if !ok {
			return client.Folder{}, fmt.Errorf("no folder has id %d — see `courrier folders`", id)
		}
		return folder, nil
	}
	var matches []client.Folder
	for _, folder := range folders {
		if strings.EqualFold(folder.Name, ref) || strings.EqualFold(folder.Path, ref) {
			matches = append(matches, folder)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return client.Folder{}, fmt.Errorf("no folder is named %q — see `courrier folders`", ref)
	}
	return client.Folder{}, fmt.Errorf("%d folders are named %q — pass the folder id instead", len(matches), ref)
}
