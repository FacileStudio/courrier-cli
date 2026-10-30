package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

var sinceDuration = regexp.MustCompile(`^(\d+)([mhdw])$`)

var sinceUnits = map[string]time.Duration{
	"m": time.Minute,
	"h": time.Hour,
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
}

// parseSince turns the --since grammar into a cutoff: a duration counted back
// from now (30m, 2h, 1d, 1w) or an RFC 3339 timestamp.
func parseSince(raw string, now time.Time) (time.Time, error) {
	if match := sinceDuration.FindStringSubmatch(raw); match != nil {
		count, err := strconv.Atoi(match[1])
		if err == nil {
			return now.Add(-time.Duration(count) * sinceUnits[match[2]]), nil
		}
	}
	if at, ok := parseDate(raw); ok {
		return at, nil
	}
	return time.Time{}, fmt.Errorf("invalid --since %q — use a duration such as 30m, 2h, 1d or 1w, or an RFC 3339 time", raw)
}

// applySince drops the rows older than the cutoff, client-side, from a listing
// the server cannot filter by date. Rows come newest first, so once any row is
// dropped no later page holds a newer one and the total is closed off there.
// A row whose date cannot be read is kept rather than hidden.
func applySince(page *client.EmailPage, cutoff time.Time) {
	kept := make([]client.Email, 0, len(page.Emails))
	for _, email := range page.Emails {
		if at, ok := parseDate(email.Date); !ok || !at.Before(cutoff) {
			kept = append(kept, email)
		}
	}
	if len(kept) < len(page.Emails) {
		page.Total = (max(page.Page, 1)-1)*max(page.Limit, len(page.Emails)) + len(kept)
	}
	page.Emails = kept
}
