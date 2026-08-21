// Package client talks to a Courrier instance's HTTP API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Error carries the API's own error code alongside its message, so a caller can
// branch on the code rather than parse prose.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Unauthenticated reports whether the instance rejected the session.
func (e *Error) Unauthenticated() bool { return e.Status == http.StatusUnauthorized }

// NotFound reports whether the instance has no such resource — or, on a login,
// no such route.
func (e *Error) NotFound() bool { return e.Status == http.StatusNotFound }

// Client is a connection to one instance.
type Client struct {
	BaseURL string
	Token   string
	http    *http.Client
}

// New builds a client. The timeout is generous because an IMAP sync or a send
// over a slow relay can take a while, but bounded so a hung server is never
// waited on forever.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// newRequest builds a request carrying the session and the client's identity.
//
// The session travels as a bearer token and never as a cookie. porte reads the
// cookie first and refuses a cookie-authenticated mutating request without an
// X-Facile-CSRF header, which would break every write while leaving every read
// working. Nothing attaches a bearer header on the caller's behalf, so bearer
// is exempt from that rule by construction.
func (c *Client) newRequest(ctx context.Context, method, path, contentType string, payload io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, payload)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set("User-Agent", "courrier-cli")
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return request, nil
}

// request builds a JSON request, encoding body when there is one.
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	return c.newRequest(ctx, method, path, "application/json", payload)
}

// send performs a prepared request and decodes the response into out, if given.
//
// The body is read whole and parsed deliberately rather than through a
// streaming decoder: Courrier serves its SvelteKit dashboard from the same
// origin behind an SPA catch-all, so a mistyped path returns 200 and HTML, and
// a bare JSON syntax error would hide that the URL was simply wrong.
func (c *Client) send(request *http.Request, out any) error {
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("cannot reach %s — %w", c.BaseURL, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeError(raw, response.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s answered with something that is not JSON — check the URL points at a Courrier instance", request.URL)
	}
	return nil
}

// do is the JSON round trip: build, send, decode.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	request, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	return c.send(request, out)
}

// decodeError unpacks the suite's error envelope, falling back to the status
// line when the body is not one — which is what a plain-text 404 or an HTML
// error page from a proxy looks like.
func decodeError(raw []byte, status int) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Message != "" {
		return &Error{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	return &Error{Status: status, Code: "unknown", Message: fmt.Sprintf("HTTP %d", status)}
}

// AuthConfig asks the instance what it accepts before a login is attempted.
func (c *Client) AuthConfig(ctx context.Context) (AuthConfig, error) {
	var out AuthConfig
	err := c.do(ctx, http.MethodGet, "/api/auth/config", nil, &out)
	return out, err
}

// Exchange trades a one-time porte login code for a session token. The code is
// valid for sixty seconds and works once.
func (c *Client) Exchange(ctx context.Context, code string) (string, error) {
	var exchanged struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}
	err := c.do(ctx, http.MethodPost, "/api/auth/oidc/exchange", struct {
		Code string `json:"code"`
	}{code}, &exchanged)
	if err != nil {
		return "", err
	}
	if exchanged.Token == "" {
		return "", fmt.Errorf("the instance returned an empty token for the login code")
	}
	return exchanged.Token, nil
}

// PasswordLogin exchanges an address and a password for a session token.
//
// A 404 here is not a missing endpoint: under SSO_ONLY porte does not register
// the local credential routes at all, so the whole path is absent. The returned
// error is an *Error whose NotFound reports true, which is how a caller tells
// "this instance has no password login" apart from "these credentials are
// wrong" — the latter arrives as 401.
func (c *Client) PasswordLogin(ctx context.Context, email, password string) (string, error) {
	var issued struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}
	body := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{email, password}
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", body, &issued); err != nil {
		return "", err
	}
	if issued.Token == "" {
		return "", fmt.Errorf("the instance returned an empty token for the login")
	}
	return issued.Token, nil
}

// Logout revokes the session server-side.
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/auth/logout", nil, nil)
}

// Accounts lists the mailboxes configured for the session's user.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var out struct {
		Accounts []Account `json:"accounts"`
	}
	err := c.do(ctx, http.MethodGet, "/api/accounts", nil, &out)
	return out.Accounts, err
}

// mailPath builds a path under one account's mail routes.
func mailPath(accountID int64, suffix string) string {
	return "/api/accounts/" + strconv.FormatInt(accountID, 10) + "/mail" + suffix
}

// withQuery appends an encoded query to a path, leaving the path alone when
// there is nothing to append.
func withQuery(path string, query url.Values) string {
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

// listValues encodes the paging options, omitting zero values so the server
// applies its own defaults and caps.
func listValues(opts ListOptions) url.Values {
	query := url.Values{}
	if opts.Page > 0 {
		query.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.Limit > 0 {
		query.Set("limit", strconv.Itoa(opts.Limit))
	}
	return query
}

// SyncAccount pulls new mail for every folder on the account. Rate limited to
// five calls a minute.
func (c *Client) SyncAccount(ctx context.Context, accountID int64) error {
	var out struct {
		Synced bool `json:"synced"`
	}
	return c.do(ctx, http.MethodPost, mailPath(accountID, "/sync"), nil, &out)
}

// Folders lists the account's mailboxes with their unread and total counts.
func (c *Client) Folders(ctx context.Context, accountID int64) ([]Folder, error) {
	var out struct {
		Folders []Folder `json:"folders"`
	}
	err := c.do(ctx, http.MethodGet, mailPath(accountID, "/folders"), nil, &out)
	return out.Folders, err
}

// SyncFolder pulls new mail for one folder, addressed by folder ID. Rate
// limited to ten calls a minute.
func (c *Client) SyncFolder(ctx context.Context, accountID, folderID int64) error {
	path := mailPath(accountID, "/folders/"+strconv.FormatInt(folderID, 10)+"/sync")
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// ListEmails returns one page of a folder, as collapsed conversations.
//
// folderType is a folder TYPE — inbox, sent, drafts, trash, junk, archive or
// custom — and never a folder ID, which is the one place in this API where the
// two are addressed differently. The rows are threads: see Email for the fields
// that only appear here.
func (c *Client) ListEmails(ctx context.Context, accountID int64, folderType string, opts ListOptions) (EmailPage, error) {
	query := listValues(opts)
	if opts.UnreadOnly {
		query.Set("unread", "true")
	}
	path := mailPath(accountID, "/folders/"+url.PathEscape(folderType)+"/emails")

	var out EmailPage
	err := c.do(ctx, http.MethodGet, withQuery(path, query), nil, &out)
	return out, err
}

// Thread expands one conversation into its messages, oldest first.
//
// threadID is a Message-ID, which routinely contains the slashes, question
// marks and hashes that a raw path would swallow, so it is percent-encoded
// before the path is built and decoded again server-side.
func (c *Client) Thread(ctx context.Context, accountID int64, threadID string) ([]Email, error) {
	var out struct {
		Emails []Email `json:"emails"`
	}
	path := mailPath(accountID, "/threads/"+url.PathEscape(threadID))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Emails, err
}

// Search matches q against subject, sender and text body, skipping trash and
// junk. A blank q is answered with an empty page rather than an error.
func (c *Client) Search(ctx context.Context, accountID int64, q string, opts ListOptions) (EmailPage, error) {
	query := listValues(opts)
	query.Set("q", q)

	var out EmailPage
	err := c.do(ctx, http.MethodGet, withQuery(mailPath(accountID, "/search"), query), nil, &out)
	return out, err
}

// Email fetches one message with its bodies and attachment metadata.
func (c *Client) Email(ctx context.Context, accountID, emailID int64) (Email, error) {
	var out Email
	path := mailPath(accountID, "/emails/"+strconv.FormatInt(emailID, 10))
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// UpdateEmail flips the read or starred flag and returns the updated message.
// Both arguments are optional: a nil pointer leaves that flag alone.
func (c *Client) UpdateEmail(ctx context.Context, accountID, emailID int64, isRead, isStarred *bool) (Email, error) {
	body := struct {
		IsRead    *bool `json:"is_read,omitempty"`
		IsStarred *bool `json:"is_starred,omitempty"`
	}{isRead, isStarred}

	var out Email
	path := mailPath(accountID, "/emails/"+strconv.FormatInt(emailID, 10))
	err := c.do(ctx, http.MethodPatch, path, body, &out)
	return out, err
}

// maxBulkIDs is the server's ceiling for one bulk action.
const maxBulkIDs = 200

// BulkAction applies one action to a batch of messages. Valid actions are
// delete, archive, mark_read and mark_unread. Rate limited to thirty calls a
// minute.
//
// An empty batch and a batch over maxBulkIDs are both a 400 server-side, so
// they are refused here instead of spending a round trip to be told so.
func (c *Client) BulkAction(ctx context.Context, accountID int64, ids []int64, action string) error {
	if len(ids) == 0 {
		return fmt.Errorf("a bulk %s needs at least one email id", action)
	}
	if len(ids) > maxBulkIDs {
		return fmt.Errorf("a bulk %s takes at most %d email ids, got %d", action, maxBulkIDs, len(ids))
	}

	body := struct {
		EmailIDs []int64 `json:"email_ids"`
		Action   string  `json:"action"`
	}{ids, action}
	var out struct {
		OK bool `json:"ok"`
	}
	return c.do(ctx, http.MethodPost, mailPath(accountID, "/emails/bulk-action"), body, &out)
}

// sendPayload is the JSON shape of an outgoing message, used when there is
// nothing to attach.
type sendPayload struct {
	To         []string `json:"to"`
	Cc         []string `json:"cc"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
	BodyHTML   string   `json:"body_html"`
	InReplyTo  string   `json:"in_reply_to"`
	References []string `json:"references"`
}

// Send delivers a message over SMTP and appends it to the account's Sent
// folder. Rate limited to ten calls a minute.
//
// With no attachments the request is JSON. With attachments it is
// multipart/form-data, where the server reads to, cc and references as
// comma-separated strings rather than repeated fields, and caps its parse at
// 25 MB.
func (c *Client) Send(ctx context.Context, accountID int64, req SendRequest) error {
	path := mailPath(accountID, "/send")
	if len(req.Attachments) == 0 {
		body := sendPayload{
			To:         req.To,
			Cc:         req.Cc,
			Subject:    req.Subject,
			Body:       req.Body,
			BodyHTML:   req.BodyHTML,
			InReplyTo:  req.InReplyTo,
			References: req.References,
		}
		var out struct {
			Sent bool `json:"sent"`
		}
		return c.do(ctx, http.MethodPost, path, body, &out)
	}

	payload, contentType, err := encodeSendMultipart(req)
	if err != nil {
		return err
	}
	request, err := c.newRequest(ctx, http.MethodPost, path, contentType, payload)
	if err != nil {
		return err
	}
	var out struct {
		Sent bool `json:"sent"`
	}
	return c.send(request, &out)
}

// encodeSendMultipart builds the multipart body for a message with
// attachments, buffered whole because the server refuses anything over 25 MB
// anyway. Empty fields are left out rather than sent blank, since the server
// treats a present-but-empty to or cc as no recipients either way.
func encodeSendMultipart(req SendRequest) (io.Reader, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	fields := [][2]string{
		{"to", strings.Join(req.To, ",")},
		{"cc", strings.Join(req.Cc, ",")},
		{"subject", req.Subject},
		{"body", req.Body},
		{"body_html", req.BodyHTML},
		{"in_reply_to", req.InReplyTo},
		{"references", strings.Join(req.References, ",")},
	}
	for _, field := range fields {
		if field[1] == "" {
			continue
		}
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", err
		}
	}

	for _, path := range req.Attachments {
		file, err := os.Open(path)
		if err != nil {
			return nil, "", fmt.Errorf("cannot attach %s — %w", path, err)
		}
		part, err := writer.CreateFormFile("attachments", filepath.Base(path))
		if err != nil {
			file.Close()
			return nil, "", err
		}
		_, err = io.Copy(part, file)
		file.Close()
		if err != nil {
			return nil, "", fmt.Errorf("cannot attach %s — %w", path, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &buffer, writer.FormDataContentType(), nil
}
