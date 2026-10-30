package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveDraftPostsJSONAndReturnsID(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.Method + " " + r.URL.Path
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{"id":42}`)
	}))
	defer server.Close()

	id, err := New(server.URL, "t").SaveDraft(context.Background(), 3, SendRequest{
		To: []string{"a@b.c"}, Subject: "s", BodyHTML: "<p>x</p>", References: []string{"<r>"},
	})
	if err != nil || id != 42 {
		t.Fatalf("SaveDraft = (%d, %v), want (42, nil)", id, err)
	}
	if path != "POST /api/accounts/3/mail/drafts" {
		t.Errorf("request = %q", path)
	}
	if body["body_html"] != "<p>x</p>" || body["subject"] != "s" {
		t.Errorf("body = %v", body)
	}
}

func TestDeleteDraftAndTemplatesRoutes(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		io.WriteString(w, `{"templates":[{"id":1,"name":"n"}],"id":1,"name":"n","deleted":true}`)
	}))
	defer server.Close()
	api, ctx := New(server.URL, "t"), context.Background()

	if err := api.DeleteDraft(ctx, 3, 9); err != nil {
		t.Fatal(err)
	}
	list, err := api.Templates(ctx, "sp")
	if err != nil || len(list) != 1 {
		t.Fatalf("Templates = (%v, %v)", list, err)
	}
	if _, err := api.CreateTemplate(ctx, TemplateInput{Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.UpdateTemplate(ctx, 1, TemplateInput{Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteTemplate(ctx, 1); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"DELETE /api/accounts/3/mail/drafts/9",
		"GET /api/templates/?space_id=sp",
		"POST /api/templates/",
		"PUT /api/templates/1",
		"DELETE /api/templates/1",
	}
	for i, w := range want {
		if i >= len(seen) || seen[i] != w {
			t.Errorf("request %d = %q, want %q", i, seen[i], w)
		}
	}
}

func TestDownloadAttachmentReturnsRawBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/1/mail/emails/2/attachments/3/download" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte{0, 1, 2, 255})
	}))
	defer server.Close()

	raw, err := New(server.URL, "t").DownloadAttachment(context.Background(), 1, 2, 3)
	if err != nil || len(raw) != 4 || raw[3] != 255 {
		t.Fatalf("DownloadAttachment = (%v, %v)", raw, err)
	}
	if _, err := New(server.URL, "t").DownloadAttachment(context.Background(), 1, 2, 4); err == nil {
		t.Error("a 404 must be an error")
	}
}
