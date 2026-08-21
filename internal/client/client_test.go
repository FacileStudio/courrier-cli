package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestCarriesBearerAndNoCookie(t *testing.T) {
	var got *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		io.WriteString(w, `{"accounts":[]}`)
	}))
	defer server.Close()

	if _, err := New(server.URL, "tok-123").Accounts(context.Background()); err != nil {
		t.Fatalf("Accounts: %v", err)
	}

	if want := "Bearer tok-123"; got.Header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", got.Header.Get("Authorization"), want)
	}
	if want := "courrier-cli"; got.Header.Get("User-Agent") != want {
		t.Errorf("User-Agent = %q, want %q", got.Header.Get("User-Agent"), want)
	}
	if cookie := got.Header.Get("Cookie"); cookie != "" {
		t.Errorf("Cookie = %q, want none: porte refuses cookie-authenticated writes without a CSRF header", cookie)
	}
}

func TestErrorEnvelopeDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"code":"unauthorized","message":"nope"}}`)
	}))
	defer server.Close()

	err := New(server.URL, "tok").Logout(context.Background())

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *Error", err, err)
	}
	if !apiErr.Unauthenticated() {
		t.Errorf("Unauthenticated() = false, want true (status %d)", apiErr.Status)
	}
	if apiErr.NotFound() {
		t.Error("NotFound() = true on a 401")
	}
	if apiErr.Code != "unauthorized" || apiErr.Message != "nope" {
		t.Errorf("got code %q message %q, want \"unauthorized\" / \"nope\"", apiErr.Code, apiErr.Message)
	}
}

func TestPasswordLoginNotFoundStaysDistinguishable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "404 page not found\n")
	}))
	defer server.Close()

	_, err := New(server.URL, "").PasswordLogin(context.Background(), "a@b.c", "pw")

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *Error", err, err)
	}
	if !apiErr.NotFound() {
		t.Errorf("NotFound() = false, want true so the caller can report SSO_ONLY (status %d)", apiErr.Status)
	}
}

func TestHTMLResponseMentionsTheURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!doctype html><html><body>Courrier</body></html>")
	}))
	defer server.Close()

	_, err := New(server.URL, "tok").Accounts(context.Background())
	if err == nil {
		t.Fatal("Accounts on an HTML 200 returned no error")
	}
	if !strings.Contains(err.Error(), server.URL+"/api/accounts") {
		t.Errorf("err = %q, want it to name the URL that answered", err)
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		t.Errorf("err = %q, want a URL complaint rather than a raw JSON syntax error", err)
	}
}

func TestThreadEscapesMessageID(t *testing.T) {
	var escaped, decoded string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped, decoded = r.URL.EscapedPath(), r.URL.Path
		io.WriteString(w, `{"emails":[{"id":7}]}`)
	}))
	defer server.Close()

	threadID := "<a/b?c#d@mail.example>"
	emails, err := New(server.URL, "tok").Thread(context.Background(), 3, threadID)
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if len(emails) != 1 || emails[0].ID != 7 {
		t.Fatalf("emails = %+v, want one email with id 7", emails)
	}

	want := "/api/accounts/3/mail/threads/%3Ca%2Fb%3Fc%23d@mail.example%3E"
	if escaped != want {
		t.Errorf("escaped path = %q, want %q", escaped, want)
	}
	if decoded != "/api/accounts/3/mail/threads/"+threadID {
		t.Errorf("decoded path = %q, want the Message-ID back intact", decoded)
	}
}

func TestListEmailsQuery(t *testing.T) {
	var query string
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.RawQuery
		io.WriteString(w, `{"emails":[],"total":0,"page":2,"limit":25}`)
	}))
	defer server.Close()

	client := New(server.URL, "tok")

	page, err := client.ListEmails(context.Background(), 1, "inbox", ListOptions{Page: 2, Limit: 25})
	if err != nil {
		t.Fatalf("ListEmails: %v", err)
	}
	if page.Page != 2 || page.Limit != 25 {
		t.Errorf("page = %+v, want page 2 limit 25", page)
	}
	if path != "/api/accounts/1/mail/folders/inbox/emails" {
		t.Errorf("path = %q, want the folder type in the path", path)
	}
	if query != "limit=25&page=2" {
		t.Errorf("query = %q, want limit=25&page=2 with no unread flag", query)
	}

	if _, err := client.ListEmails(context.Background(), 1, "inbox", ListOptions{UnreadOnly: true}); err != nil {
		t.Fatalf("ListEmails unread: %v", err)
	}
	if query != "unread=true" {
		t.Errorf("query = %q, want unread=true alone", query)
	}
}

func TestBulkActionRejectsBadBatchWithoutARequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("BulkAction reached the server with a batch the server would have rejected")
	}))
	defer server.Close()

	client := New(server.URL, "tok")

	if err := client.BulkAction(context.Background(), 1, nil, "delete"); err == nil {
		t.Error("BulkAction with no ids returned no error")
	}

	tooMany := make([]int64, maxBulkIDs+1)
	if err := client.BulkAction(context.Background(), 1, tooMany, "archive"); err == nil {
		t.Errorf("BulkAction with %d ids returned no error", len(tooMany))
	}
}

func TestSendUsesJSONWithoutAttachments(t *testing.T) {
	var contentType string
	var body sendPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{"sent":true}`)
	}))
	defer server.Close()

	err := New(server.URL, "tok").Send(context.Background(), 4, SendRequest{
		To:      []string{"a@example.com", "b@example.com"},
		Subject: "hello",
		Body:    "text",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if len(body.To) != 2 || body.To[0] != "a@example.com" {
		t.Errorf("to = %v, want a JSON array of both recipients", body.To)
	}
}

func TestSendUsesMultipartWithAttachments(t *testing.T) {
	attachment := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachment, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	var contentType, to string
	var filename, contents string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			return
		}
		to = r.FormValue("to")
		files := r.MultipartForm.File["attachments"]
		if len(files) != 1 {
			t.Errorf("attachments = %d files, want 1", len(files))
			return
		}
		filename = files[0].Filename
		opened, err := files[0].Open()
		if err != nil {
			t.Errorf("open attachment: %v", err)
			return
		}
		defer opened.Close()
		raw, _ := io.ReadAll(opened)
		contents = string(raw)
		io.WriteString(w, `{"sent":true}`)
	}))
	defer server.Close()

	err := New(server.URL, "tok").Send(context.Background(), 4, SendRequest{
		To:          []string{"a@example.com", "b@example.com"},
		Subject:     "hello",
		Body:        "text",
		Attachments: []string{attachment},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", contentType)
	}
	if to != "a@example.com,b@example.com" {
		t.Errorf("to = %q, want the recipients comma-joined", to)
	}
	if filename != "note.txt" || contents != "payload" {
		t.Errorf("attachment = %q/%q, want note.txt/payload", filename, contents)
	}
}
