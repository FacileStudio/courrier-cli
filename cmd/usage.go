package cmd

import (
	"errors"
	"time"

	"github.com/spf13/cobra"
)

// usage wraps a flag check as a PreRunE that reports a bad flag as a usage
// error: the root's PersistentPreRun has already marked the command started, so
// the mark is undone and Execute exits 2. The check runs before any session is
// opened, so a bad flag is reported even when logged out.
func usage(check func() error) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		err := check()
		if err != nil {
			commandStarted = false
		}
		return err
	}
}

// checkPaging refuses a page or limit below 1 instead of ignoring it.
func checkPaging(limit, page int) error {
	if limit < 1 {
		return errors.New("--limit must be at least 1")
	}
	if page < 1 {
		return errors.New("--page must be at least 1, pages count from 1")
	}
	return nil
}

// checkSince validates a --since value when one is given.
func checkSince(raw string) error {
	if raw == "" {
		return nil
	}
	_, err := parseSince(raw, time.Now())
	return err
}

func checkListFlags() error {
	if err := checkSince(flagListSince); err != nil {
		return err
	}
	return checkPaging(flagListLimit, flagListPage)
}

func checkContactsFlags() error {
	if flagContactsAll && flagAccount != 0 {
		return errors.New("--all and --account exclude each other — pass one")
	}
	return checkPaging(flagContactsLimit, flagContactsPage)
}

func checkTriageFlags() error {
	if _, err := parseClasses(flagTriageClass); err != nil {
		return err
	}
	if flagTriageAll && flagAccount != 0 {
		return errors.New("--all and --account exclude each other — pass one")
	}
	if err := checkSince(flagTriageSince); err != nil {
		return err
	}
	return checkPaging(flagTriageLimit, flagTriagePage)
}
