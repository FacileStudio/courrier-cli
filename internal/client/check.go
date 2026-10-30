package client

import (
	"context"
	"net/http"
	"net/url"
)

// CheckLeg is one half of a connection check. Configured false means the
// account has no host for that protocol, which differs from a failed login.
type CheckLeg struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

// CheckResult is the outcome of probing an account's stored credentials.
type CheckResult struct {
	IMAP CheckLeg `json:"imap"`
	SMTP CheckLeg `json:"smtp"`
}

// CheckAccount signs in to IMAP and SMTP with the stored passwords. A refused
// login is still a 200 with ok false. Rate limited to ten calls a minute.
func (c *Client) CheckAccount(ctx context.Context, accountID int64) (CheckResult, error) {
	var out CheckResult
	err := c.do(ctx, http.MethodPost, mailPath(accountID, "/check"), nil, &out)
	return out, err
}

// SearchContacts autocompletes addresses on one account by name or email.
func (c *Client) SearchContacts(ctx context.Context, accountID int64, q string) ([]ContactMatch, error) {
	var out struct {
		Contacts []ContactMatch `json:"contacts"`
	}
	path := withQuery(mailPath(accountID, "/contacts"), url.Values{"q": {q}})
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Contacts, err
}

// ContactMatch is one autocomplete hit with how often it was used.
type ContactMatch struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Count int    `json:"count"`
}
