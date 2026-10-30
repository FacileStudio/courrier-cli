package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

const truncatedNotice = "results truncated: the server only looked at the newest 5000 conversations"

// renderTriage prints one line per conversation and a footer with the counts.
func renderTriage(out client.Triage) {
	if len(out.Items) == 0 {
		ui.Plain("No mail")
	} else {
		many := manyAccounts(out.Items)
		header := []string{"CLASS", "WHEN", "FROM", "SUBJECT"}
		if many {
			header = append(header, "ACCOUNT")
		}
		rows := make([][]string, 0, len(out.Items))
		for _, item := range out.Items {
			rows = append(rows, triageRow(item, many))
		}
		ui.Table(header, rows)
	}
	ui.Plain("%s", ui.Dim(triageFooter(out)))
	if out.Truncated {
		ui.Warn("%s", truncatedNotice)
	}
}

func triageRow(item client.TriageItem, withAccount bool) []string {
	sender := item.FromName
	if sender == "" {
		sender = item.FromAddress
	}
	subject := strings.TrimSpace(item.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	if item.MessageCount > 1 {
		subject += " (" + strconv.Itoa(item.MessageCount) + ")"
	}
	row := []string{item.Class, relativeTime(item.Date), truncate(sender, 28), truncate(subject, 56)}
	if withAccount {
		row = append(row, item.Account)
	}
	return row
}

func manyAccounts(items []client.TriageItem) bool {
	for _, item := range items {
		if item.AccountID != items[0].AccountID {
			return true
		}
	}
	return false
}

// triageFooter states the total after --class, the counts that ignore --class
// and, when the page does not hold all of it, which page this is.
func triageFooter(out client.Triage) string {
	footer := fmt.Sprintf("%d conversations  ·  %d known  ·  %d unknown_human  ·  %d bulk",
		out.Total, out.Counts.Known, out.Counts.UnknownHuman, out.Counts.Bulk)
	if out.Total > len(out.Items) {
		footer += fmt.Sprintf("  ·  page %d, --page %d for more", max(out.Page, 1), max(out.Page, 1)+1)
	}
	return footer
}
