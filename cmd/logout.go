package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// logoutCmd revokes the session server-side and then forgets it locally.
//
// The order matters and so does the tolerance: the local credential is cleared
// even when the server refuses the revocation, because somebody logging out of
// a borrowed machine cares far more about the token leaving the disk than about
// a clean round trip. Holding the file hostage to a network error would be the
// one failure mode with a real cost.
//
// Only the stored session is touched. A token supplied through COURRIER_TOKEN
// is deliberately left alone: that is usually a labelled dashboard token, of
// which Courrier keeps one per user, so revoking it here would sign out every
// other agent sharing it.
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke the stored session",
	Long: `Revokes the session token on the server and forgets it locally, keeping the
instance URL and the default mail account so the next login needs neither.

Running it with no session stored is not an error.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signalContext()
		defer stop()

		cfg, err := config.Load()
		unreadable := err != nil
		if unreadable {
			ui.Warn("the stored configuration could not be read — clearing it anyway")
			cfg = config.Config{URL: config.DefaultURL}
		}
		if cfg.Token == "" && !unreadable {
			ui.Warn("no session stored — nothing to revoke")
			warnEnvTokenWins()
			return logoutReport(cfg.URL, false)
		}

		if cfg.Token != "" {
			api := client.New(cfg.URL, cfg.Token)
			if err := api.Logout(ctx); err != nil {
				var apiErr *client.Error
				if !errors.As(err, &apiErr) || !apiErr.Unauthenticated() {
					ui.Warn("the server could not revoke the session — %s", err)
				}
			}
		}
		if err := config.Clear(); err != nil {
			return err
		}
		warnEnvTokenWins()
		return logoutReport(cfg.URL, true)
	},
}

// warnEnvTokenWins says so when an environment token is still in play. Without
// it, a logout that reports success is followed by commands that keep working,
// which reads as the logout having silently failed.
func warnEnvTokenWins() {
	if os.Getenv("COURRIER_TOKEN") != "" {
		ui.Warn("COURRIER_TOKEN is still set and outranks the stored session — unset it to finish signing out")
	}
}

// logoutReport prints the outcome, as one JSON document under --json and as a
// line of prose otherwise.
func logoutReport(url string, cleared bool) error {
	if flagJSON {
		return ui.JSON(struct {
			URL     string `json:"url"`
			Cleared bool   `json:"cleared"`
		}{url, cleared})
	}
	if cleared {
		ui.Success("signed out of %s", url)
	}
	return nil
}
