package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

// prefixed adds a subject prefix such as "Re: " once, case-insensitively.
func prefixed(prefix, subject string) string {
	trimmed := strings.TrimSpace(subject)
	if strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(prefix)) {
		return trimmed
	}
	return prefix + trimmed
}

// replyRecipients computes To and Cc for a reply. A reply to everyone adds the
// original To and Cc, minus the sender already in To and the account's own
// address.
func replyRecipients(original client.Email, all bool, self string) (to, cc []string) {
	to = []string{original.FromAddress}
	if !all {
		return to, nil
	}
	for _, address := range slices.Concat(addressList(original.ToAddresses), addressList(original.CcAddresses)) {
		if strings.EqualFold(address, self) || containsFold(to, address) || containsFold(cc, address) {
			continue
		}
		cc = append(cc, address)
	}
	return to, cc
}

// containsFold reports whether list holds address, ignoring case.
func containsFold(list []string, address string) bool {
	return slices.ContainsFunc(list, func(have string) bool { return strings.EqualFold(have, address) })
}

// threadReferences is the original's References chain plus its own Message-ID.
func threadReferences(original client.Email) []string {
	refs := strings.Fields(original.References)
	if original.MessageID != "" && !slices.Contains(refs, original.MessageID) {
		refs = append(refs, original.MessageID)
	}
	return refs
}

// quoteText prefixes every line of the original with "> ".
func quoteText(original client.Email) string {
	lines := strings.Split(strings.TrimRight(bodyOf(original), "\n"), "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	header := fmt.Sprintf("On %s, %s wrote:", original.Date, addressOf(original))
	return header + "\n" + strings.Join(lines, "\n")
}

// quoteHTML wraps the original in a blockquote, escaping plain text when the
// original has no HTML part.
func quoteHTML(original client.Email) string {
	inner := original.BodyHTML
	if inner == "" {
		inner = escapeHTML(bodyOf(original))
	}
	header := escapeHTML(fmt.Sprintf("On %s, %s wrote:", original.Date, addressOf(original)))
	return "<p>" + header + "</p>\n<blockquote>" + inner + "</blockquote>"
}

// buildReply assembles the reply request. Explicit --to and --cc replace the
// computed recipients.
func buildReply(f composeFlags, original client.Email, text, htmlBody *string, all bool, self string, cmd *cobra.Command) client.SendRequest {
	req := f.request(text, htmlBody)
	to, cc := replyRecipients(original, all, self)
	if !cmd.Flags().Changed("to") {
		req.To = to
	}
	if !cmd.Flags().Changed("cc") {
		req.Cc = cc
	}
	if !cmd.Flags().Changed("subject") {
		req.Subject = prefixed("Re: ", original.Subject)
	}
	req.InReplyTo = original.MessageID
	req.References = threadReferences(original)
	if text != nil {
		req.Body = *text + "\n\n" + quoteText(original)
	} else {
		req.Body = quoteText(original)
	}
	if htmlBody != nil {
		req.BodyHTML = *htmlBody + "\n" + quoteHTML(original)
	}
	return req
}

// forwardBlock renders the original's headers and text for a forward.
func forwardBlock(original client.Email) string {
	lines := []string{
		"---------- Forwarded message ----------",
		"From: " + addressOf(original),
		"Date: " + original.Date,
		"Subject: " + original.Subject,
		"To: " + joinAddresses(original.ToAddresses),
	}
	if cc := joinAddresses(original.CcAddresses); cc != "" {
		lines = append(lines, "Cc: "+cc)
	}
	return strings.Join(lines, "\n") + "\n\n" + strings.TrimRight(bodyOf(original), "\n")
}

// buildForward assembles the forward request around the original.
func buildForward(f composeFlags, original client.Email, text, htmlBody *string) client.SendRequest {
	req := f.request(text, htmlBody)
	if req.Subject == "" {
		req.Subject = prefixed("Fwd: ", original.Subject)
	}
	req.Body = strings.TrimLeft((req.Body)+"\n\n"+forwardBlock(original), "\n")
	if htmlBody != nil {
		req.BodyHTML = *htmlBody + "\n<p>" + escapeHTML(forwardBlock(original)) + "</p>"
	}
	return req
}
