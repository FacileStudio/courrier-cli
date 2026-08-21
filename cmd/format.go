package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// renderEmailRows prints collapsed conversations the way a folder listing
// returns them: one line per thread, with the message count only where a
// conversation actually holds more than one.
func renderEmailRows(emails []client.Email) {
	if len(emails) == 0 {
		ui.Plain("No mail")
		return
	}
	rows := make([][]string, 0, len(emails))
	for _, email := range emails {
		rows = append(rows, []string{
			strconv.FormatInt(email.ID, 10),
			unreadMarker(email),
			relativeTime(email.Date),
			truncate(senderName(email), 28),
			truncate(subjectOrPlaceholder(email), 56) + threadSuffix(email),
		})
	}
	ui.Table([]string{"ID", "", "WHEN", "FROM", "SUBJECT"}, rows)
}

// renderThread prints every message in one conversation, oldest first, which is
// the order a conversation is read in.
func renderThread(emails []client.Email) {
	for i, email := range emails {
		if i > 0 {
			ui.Plain("")
		}
		ui.Plain("%s", ui.Dim(strings.Repeat("─", 60)))
		ui.Plain("%s", subjectOrPlaceholder(email))
		ui.Plain("%s", ui.Dim(fmt.Sprintf("from %s  ·  %s  ·  id %d",
			addressOf(email), clockTime(email.Date), email.ID)))
		if to := joinAddresses(email.ToAddresses); to != "" {
			ui.Plain("%s", ui.Dim("to   "+to))
		}
		if cc := joinAddresses(email.CcAddresses); cc != "" {
			ui.Plain("%s", ui.Dim("cc   "+cc))
		}
		for _, attachment := range email.Attachments {
			ui.Plain("%s", ui.Dim(fmt.Sprintf("file %s (%s, %d bytes)",
				attachment.Filename, attachment.MimeType, attachment.Size)))
		}
		ui.Plain("")
		ui.Plain("%s", strings.TrimRight(bodyOf(email), "\n"))
	}
}

// renderFolders prints the folder list with its counts.
func renderFolders(folders []client.Folder) {
	if len(folders) == 0 {
		ui.Plain("No folders")
		return
	}
	rows := make([][]string, 0, len(folders))
	for _, folder := range folders {
		rows = append(rows, []string{
			strconv.FormatInt(folder.ID, 10),
			folder.Type,
			folder.Name,
			strconv.Itoa(folder.UnreadCount),
			strconv.Itoa(folder.TotalCount),
		})
	}
	ui.Table([]string{"ID", "TYPE", "NAME", "UNREAD", "TOTAL"}, rows)
}

// renderAccounts prints the mail accounts, marking the one commands act on
// when no --account is given.
func renderAccounts(accounts []client.Account, current int64) {
	if len(accounts) == 0 {
		ui.Plain("No accounts")
		return
	}
	rows := make([][]string, 0, len(accounts))
	for _, account := range accounts {
		marker := ""
		if account.ID == current {
			marker = "●"
		}
		rows = append(rows, []string{
			marker,
			strconv.FormatInt(account.ID, 10),
			account.Name,
			account.Email,
			account.IMAPHost,
		})
	}
	ui.Table([]string{"", "ID", "NAME", "ADDRESS", "IMAP"}, rows)
}

// bodyOf returns the message text a terminal can show. Courrier returns both a
// text and an HTML part, so a text client never needs an HTML renderer.
func bodyOf(email client.Email) string {
	if email.BodyText != "" {
		return email.BodyText
	}
	if email.BodyHTML != "" {
		return "(this message has only an HTML part)"
	}
	return "(no body)"
}

// senderName prefers the display name and falls back to the address, because a
// listing with an empty column reads as a bug.
func senderName(email client.Email) string {
	if email.FromName != "" {
		return email.FromName
	}
	return email.FromAddress
}

// addressOf renders the sender the way a mail header does.
func addressOf(email client.Email) string {
	if email.FromName == "" {
		return email.FromAddress
	}
	return fmt.Sprintf("%s <%s>", email.FromName, email.FromAddress)
}

func subjectOrPlaceholder(email client.Email) string {
	if strings.TrimSpace(email.Subject) == "" {
		return "(no subject)"
	}
	return email.Subject
}

// unreadMarker is a domain glyph, not a severity: it says what the row is, and
// carries meaning colour alone cannot.
func unreadMarker(email client.Email) string {
	switch {
	case !email.IsRead && email.IsStarred:
		return "●*"
	case !email.IsRead:
		return "●"
	case email.IsStarred:
		return "*"
	}
	return ""
}

func threadSuffix(email client.Email) string {
	if email.MessageCount > 1 {
		return ui.Dim(fmt.Sprintf(" (%d)", email.MessageCount))
	}
	return ""
}

func joinAddresses(addresses []client.Address) string {
	parts := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Name == "" {
			parts = append(parts, address.Email)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s <%s>", address.Name, address.Email))
	}
	return strings.Join(parts, ", ")
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}

// parseDate reads the timestamps Courrier emits, which are RFC 3339 with a
// variable fractional part.
func parseDate(raw string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

// relativeTime renders a timestamp as an age, which is what somebody scanning a
// mailbox actually wants, falling back to the calendar past a day.
func relativeTime(raw string) string {
	at, ok := parseDate(raw)
	if !ok {
		return raw
	}

	elapsed := time.Since(at)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	}
	return at.Local().Format("2 Jan 15:04")
}

// clockTime renders a timestamp in full, for a header where the exact moment
// matters more than the age.
func clockTime(raw string) string {
	at, ok := parseDate(raw)
	if !ok {
		return raw
	}
	return at.Local().Format("2 Jan 2006 15:04")
}

// renderPageSummary says which slice of the results is on screen, and only when
// there is more than one page of them: a listing that fits entirely does not
// need to be told that it fits.
func renderPageSummary(page client.EmailPage) {
	if len(page.Emails) == 0 || page.Total <= len(page.Emails) {
		return
	}

	number := page.Page
	if number < 1 {
		number = 1
	}
	size := page.Limit
	if size < 1 {
		size = len(page.Emails)
	}

	first := (number-1)*size + 1
	last := first + len(page.Emails) - 1

	summary := fmt.Sprintf("%d-%d of %d  ·  page %d", first, last, page.Total, number)
	if last < page.Total {
		summary += fmt.Sprintf("  ·  --page %d for more", number+1)
	}
	ui.Plain("%s", ui.Dim(summary))
}
