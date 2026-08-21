package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

const (
	// accountSyncPerMinute and folderSyncPerMinute are the instance's own
	// limits. They are stated in the messages a 429 produces, because "429"
	// tells a reader nothing about how long to wait.
	accountSyncPerMinute = 5
	folderSyncPerMinute  = 10

	// folderSyncInterval paces the per-folder pass just under the folder limit,
	// with headroom for the clock skew between the two ends. Ten a minute is one
	// call every six seconds, and a tight loop earns a 429 on the eleventh
	// folder — which then looks like a broken sync rather than an impatient one.
	folderSyncInterval = 6500 * time.Millisecond
)

var flagSyncFolder int64

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull new mail from IMAP",
	Long: `Pull new mail from the account's IMAP server into the instance.

With no flags this maps the account's folders and then syncs each one in turn.
The instance rate limits both halves, so a full pass is paced and a mailbox with
many folders takes a while. Sync a single folder with --folder instead; its ids
come from courrier folders.

A folder that fails does not stop the others, but it does fail the run: the exit
code is 1 and the folders that did not sync are named.`,
	Args: cobra.NoArgs,
	RunE: runSync,
}

// init wires the sync flags.
func init() {
	syncCmd.Flags().Int64Var(&flagSyncFolder, "folder", 0, "Sync only the folder with this id")
}

// folderOutcome is one folder's line in the --json report.
type folderOutcome struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"error,omitempty"`
}

// syncReport is the --json document for a sync run. Skipped holds the folders a
// rate limit cut the pass short of, which are neither synced nor failed and
// would otherwise vanish from the report.
type syncReport struct {
	AccountID int64           `json:"account_id"`
	Synced    []folderOutcome `json:"synced"`
	Failed    []folderOutcome `json:"failed"`
	Skipped   []folderOutcome `json:"skipped"`
}

// runSync resolves the session and the account, then syncs either one folder or
// all of them.
func runSync(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	api, cfg, err := session()
	if err != nil {
		return err
	}
	accountID, err := resolveAccount(ctx, api, cfg)
	if err != nil {
		return err
	}

	if flagSyncFolder != 0 {
		return syncOne(ctx, api, accountID, flagSyncFolder)
	}
	return syncAll(ctx, api, accountID)
}

// syncOne pulls a single folder, resolving the id against the account's folder
// list first so an unknown id is named here rather than returned as a bare 404
// from a URL the caller never sees.
func syncOne(ctx context.Context, api *client.Client, accountID, folderID int64) error {
	folders, err := api.Folders(ctx, accountID)
	if err != nil {
		return syncFailure(ctx, err, "list this account's folders", 0)
	}

	folder, ok := folderByID(folders, folderID)
	if !ok {
		return fmt.Errorf("this account has no folder with id %d — run `courrier folders` for the ids", folderID)
	}

	progress("syncing %s", folder.Name)
	if err := api.SyncFolder(ctx, accountID, folder.ID); err != nil {
		return syncFailure(ctx, err, "sync "+folder.Name, folderSyncPerMinute)
	}

	if flagJSON {
		return ui.JSON(syncReport{
			AccountID: accountID,
			Synced:    []folderOutcome{{ID: folder.ID, Name: folder.Name}},
			Failed:    []folderOutcome{},
			Skipped:   []folderOutcome{},
		})
	}
	ui.Success("synced %s", folder.Name)
	return nil
}

// syncAll maps the account's folders and then walks them, one paced call at a
// time.
//
// A failure anywhere in the walk fails the run, so the folders that did not take
// are collected and named at the end rather than reported as a warning nobody
// checks the exit code against. A 429 stops the walk outright: every remaining
// call would be refused too, and hammering a rate limiter only lengthens the
// wait it imposes.
func syncAll(ctx context.Context, api *client.Client, accountID int64) error {
	progress("mapping folders")
	if err := api.SyncAccount(ctx, accountID); err != nil {
		return syncFailure(ctx, err, "map this account's folders", accountSyncPerMinute)
	}

	folders, err := api.Folders(ctx, accountID)
	if err != nil {
		return syncFailure(ctx, err, "list this account's folders", 0)
	}
	if len(folders) == 0 {
		ui.Warn("this account has no folders — check its IMAP settings in the dashboard")
	}
	if len(folders) > 1 && !flagJSON {
		ui.Hint("pacing to the instance's %d folder syncs a minute", folderSyncPerMinute)
	}

	report := syncReport{
		AccountID: accountID,
		Synced:    make([]folderOutcome, 0, len(folders)),
		Failed:    []folderOutcome{},
		Skipped:   []folderOutcome{},
	}

	for i, folder := range folders {
		if i > 0 {
			if err := pause(ctx, folderSyncInterval); err != nil {
				return err
			}
		}

		progress("syncing %s (%d/%d)", folder.Name, i+1, len(folders))
		err := api.SyncFolder(ctx, accountID, folder.ID)
		if err == nil {
			report.Synced = append(report.Synced, folderOutcome{ID: folder.ID, Name: folder.Name})
			continue
		}
		if ctx.Err() != nil {
			return ErrInterrupted
		}

		failure := syncFailure(ctx, err, "sync "+folder.Name, folderSyncPerMinute)
		ui.Warn("%s", failure)
		report.Failed = append(report.Failed, folderOutcome{
			ID:     folder.ID,
			Name:   folder.Name,
			Reason: failure.Error(),
		})

		if rateLimited(err) {
			for _, remaining := range folders[i+1:] {
				report.Skipped = append(report.Skipped, folderOutcome{ID: remaining.ID, Name: remaining.Name})
			}
			break
		}
	}

	if flagJSON {
		if err := ui.JSON(report); err != nil {
			return err
		}
	} else {
		ui.Success("synced %d of %d folders", len(report.Synced), len(folders))
	}

	if len(report.Failed) > 0 {
		return fmt.Errorf("%d of %d folders did not sync (%s) — retry one with `courrier sync --folder <id>`",
			len(report.Failed), len(folders), outcomeNames(report.Failed))
	}
	return nil
}

// syncFailure turns a client error into something a reader can act on.
//
// A cancelled context is the user's own Ctrl-C and must exit 130, not 1. A 429
// is a rate limit whose only fix is waiting, and saying so plainly — with the
// limit that was hit — is the difference between a retry in a minute and a bug
// report. perMinute is the limit for this call, or zero for a call the instance
// does not limit.
func syncFailure(ctx context.Context, err error, action string, perMinute int) error {
	switch {
	case ctx.Err() != nil:
		return ErrInterrupted
	case rateLimited(err) && perMinute > 0:
		return fmt.Errorf("cannot %s — the instance allows %d of these a minute and refused this one; wait a minute and run it again", action, perMinute)
	case rateLimited(err):
		return fmt.Errorf("cannot %s — the instance is rate limiting this call; wait a minute and run it again", action)
	}
	return fmt.Errorf("cannot %s — %w", action, err)
}

// rateLimited reports whether the instance refused the call under a rate limit.
func rateLimited(err error) bool {
	var apiErr *client.Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests
}

// pause waits between paced calls, returning ErrInterrupted rather than sleeping
// through a Ctrl-C the user is waiting to take effect.
func pause(ctx context.Context, every time.Duration) error {
	timer := time.NewTimer(every)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ErrInterrupted
	case <-timer.C:
		return nil
	}
}

// progress announces a step, unless --json is set: structured output puts one
// document on stdout and nothing else, so progress is dropped rather than
// interleaved with it. Warnings and errors are unaffected — they go to stderr.
func progress(format string, a ...any) {
	if flagJSON {
		return
	}
	ui.Step(format, a...)
}

// folderByID finds a folder in a listing.
func folderByID(folders []client.Folder, id int64) (client.Folder, bool) {
	for _, folder := range folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return client.Folder{}, false
}

// outcomeNames lists folder names for an error that has to say which ones.
func outcomeNames(outcomes []folderOutcome) string {
	names := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		names = append(names, outcome.Name)
	}
	return strings.Join(names, ", ")
}
