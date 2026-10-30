package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const triageBody = `{"generated_at":"2026-10-30T10:00:00Z","total":18,"counts":{"known":2,"unknown_human":1,"bulk":15},` +
	`"items":[{"account_id":1,"account":"yann@facile.studio","thread_id":"<a@b>","email_id":1880,` +
	`"subject":"Railway security update","from_address":"hello@notify.railway.app","from_name":"Railway",` +
	`"date":"2026-10-23T20:41:41Z","message_count":1,"unread_count":1,"class":"bulk","reason":"list-unsubscribe"}],` +
	`"page":1,"limit":50}`

func fixture(t *testing.T, status int, body string, got *url.URL) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			*got = *r.URL
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "tok")
}

func TestTriageQueryAndDecoding(t *testing.T) {
	var seen url.URL
	api := fixture(t, http.StatusOK, triageBody, &seen)

	out, err := api.Triage(context.Background(), TriageQuery{
		Unread: true, Classes: []string{"known", "bulk"}, Since: "2h", Limit: 20, Page: 2, Account: 3,
	})
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}

	want := url.Values{"unread": {"true"}, "class": {"known,bulk"}, "since": {"2h"}, "limit": {"20"}, "page": {"2"}, "account": {"3"}}
	if seen.Path != "/api/mail/triage" || seen.Query().Encode() != want.Encode() {
		t.Errorf("requested %s?%s, want /api/mail/triage?%s", seen.Path, seen.RawQuery, want.Encode())
	}
	if out.Total != 18 || out.Counts.Bulk != 15 || out.Counts.Known != 2 || out.Counts.UnknownHuman != 1 {
		t.Errorf("totals decoded as %+v", out)
	}
	item := out.Items[0]
	if item.Class != "bulk" || item.Reason != "list-unsubscribe" || item.EmailID != 1880 || item.ThreadID != "<a@b>" {
		t.Errorf("item decoded as %+v", item)
	}
	if string(out.Raw) != triageBody {
		t.Error("Raw is not the response body")
	}
}

func TestTriageOmitsZeroQuery(t *testing.T) {
	var seen url.URL
	api := fixture(t, http.StatusOK, triageBody, &seen)
	if _, err := api.Triage(context.Background(), TriageQuery{}); err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if seen.RawQuery != "" {
		t.Errorf("query = %q, want none", seen.RawQuery)
	}
}

func TestContactsQueryAndDecoding(t *testing.T) {
	var seen url.URL
	body := `{"contacts":[{"email":"a@b.c","name":"A","sent_count":4,"last_sent_at":"2026-10-01T08:00:00Z"}],"total":1,"page":1,"limit":50}`
	api := fixture(t, http.StatusOK, body, &seen)

	out, err := api.Contacts(context.Background(), ContactsQuery{Limit: 10, Page: 3, Account: 2})
	if err != nil {
		t.Fatalf("Contacts: %v", err)
	}
	if seen.Path != "/api/mail/contacts" || seen.Query().Encode() != "account=2&limit=10&page=3" {
		t.Errorf("requested %s?%s", seen.Path, seen.RawQuery)
	}
	if got := out.Contacts[0]; got.Email != "a@b.c" || got.SentCount != 4 || got.LastSentAt != "2026-10-01T08:00:00Z" {
		t.Errorf("contact decoded as %+v", got)
	}
}

func TestNotFoundMapsToUnsupported(t *testing.T) {
	api := fixture(t, http.StatusNotFound, `{"error":{"code":"not_found","message":"no route"}}`, nil)

	if _, err := api.Triage(context.Background(), TriageQuery{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Triage err = %v, want ErrUnsupported", err)
	}
	if _, err := api.Contacts(context.Background(), ContactsQuery{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Contacts err = %v, want ErrUnsupported", err)
	}
	if !strings.Contains(ErrUnsupported.Error(), "upgrade the server") {
		t.Errorf("message = %q", ErrUnsupported)
	}
}

func TestOtherErrorsStayTyped(t *testing.T) {
	api := fixture(t, http.StatusUnauthorized, `{"error":{"code":"unauthorized","message":"nope"}}`, nil)
	_, err := api.Triage(context.Background(), TriageQuery{})
	var apiErr *Error
	if errors.Is(err, ErrUnsupported) || !errors.As(err, &apiErr) || !apiErr.Unauthenticated() {
		t.Errorf("err = %v, want the 401 *Error", err)
	}
}
