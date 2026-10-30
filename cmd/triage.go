package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var triageClasses = []string{"known", "unknown_human", "bulk"}

var (
	flagTriageAll    bool
	flagTriageUnread bool
	flagTriageClass  string
	flagTriageSince  string
	flagTriageLimit  int
	flagTriagePage   int
)

// triageCmd lists conversations across accounts with the class the server
// resolved for each, so a caller never has to guess which mail is automated.
var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "List conversations across accounts, classified",
	Long: `List conversations across accounts, each tagged with a class the server decided:

  known           a person you have written to
  unknown_human   looks personal, no history with you
  bulk            automated or marketing mail

With no --account and no default account, every account is covered. With a
default account set, pass --all to cover them all. The counts in the footer
cover the whole filtered set, not only the page on screen.

--since takes a duration (30m, 2h, 1d, 1w) or an RFC 3339 time. --json prints the
API response and nothing else. An instance without triage exits 1.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return runTriage() },
}

// init registers the triage flags.
func init() {
	flags := triageCmd.Flags()
	flags.BoolVar(&flagTriageAll, "all", false, "Cover every account, even when a default is set")
	flags.BoolVar(&flagTriageUnread, "unread", false, "Show only conversations holding unread mail")
	flags.StringVar(&flagTriageClass, "class", "", "Classes to show, comma separated: known, unknown_human, bulk")
	flags.StringVar(&flagTriageSince, "since", "", "Only mail newer than 2h, 1d or an RFC 3339 time")
	flags.IntVar(&flagTriageLimit, "limit", 50, "Conversations per page, at most 100")
	flags.IntVar(&flagTriagePage, "page", 1, "Page to show, counting from 1")
}

// parseClasses checks a comma list against the three classes the server knows,
// so a typo is refused here instead of silently matching nothing.
func parseClasses(raw string) ([]string, error) {
	var classes []string
	for part := range strings.SplitSeq(raw, ",") {
		class := strings.ToLower(strings.TrimSpace(part))
		if class == "" {
			continue
		}
		if !slices.Contains(triageClasses, class) {
			return nil, fmt.Errorf("unknown class %q — use known, unknown_human or bulk", class)
		}
		if !slices.Contains(classes, class) {
			classes = append(classes, class)
		}
	}
	return classes, nil
}

// scopeAccount returns the account a multi-account command is limited to, or
// zero for all of them: --all always means all, otherwise a flag, the
// environment or the stored default narrows it.
func scopeAccount(cfg config.Config, all bool) (int64, error) {
	if all && flagAccount != 0 {
		return 0, errors.New("--all and --account exclude each other — pass one")
	}
	if all {
		return 0, nil
	}
	return configuredAccount(cfg)
}

// triageQuery validates the flags and builds the API query from them.
func triageQuery(cfg config.Config) (client.TriageQuery, error) {
	classes, err := parseClasses(flagTriageClass)
	if err != nil {
		return client.TriageQuery{}, err
	}
	if flagTriageSince != "" {
		if _, err := parseSince(flagTriageSince, time.Now()); err != nil {
			return client.TriageQuery{}, err
		}
	}
	account, err := scopeAccount(cfg, flagTriageAll)
	return client.TriageQuery{
		Unread:  flagTriageUnread,
		Classes: classes,
		Since:   flagTriageSince,
		Limit:   clampLimit(flagTriageLimit),
		Page:    flagTriagePage,
		Account: account,
	}, err
}

// runTriage fetches one page and prints it.
func runTriage() error {
	ctx, stop := signalContext()
	defer stop()

	api, cfg, err := session()
	if err != nil {
		return err
	}
	query, err := triageQuery(cfg)
	if err != nil {
		return err
	}
	out, err := api.Triage(ctx, query)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if flagJSON {
		return ui.JSON(out.Raw)
	}
	renderTriage(out)
	return nil
}
