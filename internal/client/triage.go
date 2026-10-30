package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrUnsupported is matched by errors.Is against an UnsupportedError: the
// instance lacks a route, which is how a caller tells an older Courrier from a
// failing one.
var ErrUnsupported = errors.New("this Courrier instance does not support this feature; upgrade the server")

// UnsupportedError names the feature an older instance lacks.
type UnsupportedError struct{ Feature string }

func (e *UnsupportedError) Error() string {
	return "this Courrier instance does not support " + e.Feature + "; upgrade the server"
}

// Is makes an UnsupportedError match ErrUnsupported.
func (e *UnsupportedError) Is(target error) bool { return target == ErrUnsupported }

// notJSONError is a 2xx answer that is not JSON, such as the dashboard SPA fallback.
type notJSONError struct{ url string }

func (e *notJSONError) Error() string {
	return e.url + " answered with something that is not JSON — check the URL points at a Courrier instance"
}

// TriageCounts tallies conversations by class. They follow unread, since and
// account but not the class filter, and cover more than the page.
type TriageCounts struct {
	Known        int `json:"known"`
	UnknownHuman int `json:"unknown_human"`
	Bulk         int `json:"bulk"`
}

// TriageItem is one conversation with the class the server resolved for it.
type TriageItem struct {
	AccountID    int64  `json:"account_id"`
	Account      string `json:"account"`
	ThreadID     string `json:"thread_id"`
	EmailID      int64  `json:"email_id"`
	Subject      string `json:"subject"`
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name"`
	Date         string `json:"date"`
	MessageCount int    `json:"message_count"`
	UnreadCount  int    `json:"unread_count"`
	Class        string `json:"class"`
	Reason       string `json:"reason"`
}

// Triage is one page of classified conversations across accounts. Raw holds
// the response body as received, so a caller can print what the API sent.
type Triage struct {
	GeneratedAt string          `json:"generated_at"`
	Total       int             `json:"total"`
	Counts      TriageCounts    `json:"counts"`
	Items       []TriageItem    `json:"items"`
	Page        int             `json:"page"`
	Limit       int             `json:"limit"`
	Truncated   bool            `json:"truncated"`
	Raw         json.RawMessage `json:"-"`
}

// TriageQuery filters a triage call. Zero values are left off the query so the
// server applies its own defaults. Since is passed through as typed: RFC 3339
// or a duration such as 2h or 1d.
type TriageQuery struct {
	Unread  bool
	Classes []string
	Since   string
	Limit   int
	Page    int
	Account int64
}

// Contact is an address the user has written to.
type Contact struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	SentCount  int    `json:"sent_count"`
	LastSentAt string `json:"last_sent_at"`
}

// ContactPage is one page of contacts, newest recipient first. Raw holds the
// response body as received.
type ContactPage struct {
	Contacts []Contact       `json:"contacts"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	Limit    int             `json:"limit"`
	Raw      json.RawMessage `json:"-"`
}

// ContactsQuery filters a contacts call. Zero values are left off the query.
type ContactsQuery struct {
	Limit   int
	Page    int
	Account int64
}

func (q TriageQuery) values() url.Values {
	query := listValues(ListOptions{Page: q.Page, Limit: q.Limit})
	if q.Unread {
		query.Set("unread", "true")
	}
	if len(q.Classes) > 0 {
		query.Set("class", strings.Join(q.Classes, ","))
	}
	if q.Since != "" {
		query.Set("since", q.Since)
	}
	if q.Account != 0 {
		query.Set("account", strconv.FormatInt(q.Account, 10))
	}
	return query
}

func (q ContactsQuery) values() url.Values {
	query := listValues(ListOptions{Page: q.Page, Limit: q.Limit})
	if q.Account != 0 {
		query.Set("account", strconv.FormatInt(q.Account, 10))
	}
	return query
}

// getRaw fetches a route that older instances lack, keeping the body for the
// caller. An old server answers such a route with a plain-text 404 or a 200 of
// HTML; both map onto an UnsupportedError. A JSON error envelope is a real API
// answer, a wrong account id for one, and surfaces as itself.
func (c *Client) getRaw(ctx context.Context, feature, path string, out any) (json.RawMessage, error) {
	var raw json.RawMessage
	err := c.do(ctx, http.MethodGet, path, nil, &raw)
	var apiErr *Error
	var notJSON *notJSONError
	if errors.As(err, &notJSON) || errors.As(err, &apiErr) && apiErr.NotFound() && apiErr.Code == "unknown" {
		return nil, &UnsupportedError{Feature: feature}
	}
	if err != nil {
		return nil, err
	}
	return raw, json.Unmarshal(raw, out)
}

// Triage lists classified conversations across the user's accounts.
func (c *Client) Triage(ctx context.Context, q TriageQuery) (Triage, error) {
	var out Triage
	raw, err := c.getRaw(ctx, "triage", withQuery("/api/mail/triage", q.values()), &out)
	out.Raw = raw
	return out, err
}

// Contacts lists the addresses the user has written to, across accounts.
func (c *Client) Contacts(ctx context.Context, q ContactsQuery) (ContactPage, error) {
	var out ContactPage
	raw, err := c.getRaw(ctx, "contacts", withQuery("/api/mail/contacts", q.values()), &out)
	out.Raw = raw
	return out, err
}
