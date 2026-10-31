package client

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type seenRequest struct {
	method string
	uri    string
	ctype  string
	body   []byte
}

func recording(t *testing.T, response string) (*Client, *[]seenRequest) {
	t.Helper()
	var seen []seenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen = append(seen, seenRequest{r.Method, r.URL.RequestURI(), r.Header.Get("Content-Type"), raw})
		io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "tok"), &seen
}

func formOf(t *testing.T, req seenRequest) (map[string]string, []string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(req.ctype)
	if err != nil {
		t.Fatalf("content type %q: %v", req.ctype, err)
	}
	reader := multipart.NewReader(strings.NewReader(string(req.body)), params["boundary"])
	fields, files := map[string]string{}, []string{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			return fields, files
		}
		data, _ := io.ReadAll(part)
		if part.FileName() != "" {
			files = append(files, part.FileName()+"="+string(data))
		} else {
			fields[part.FormName()] = string(data)
		}
	}
}

func TestEmailKeepsEveryFieldTheServerReturns(t *testing.T) {
	full := `{"id":1,"account_id":2,"folder_id":3,"message_id":"<m>","thread_id":"<t>","subject":"s",` +
		`"from_address":"a@b.c","from_name":"A","to_addresses":[{"name":"T","email":"t@b.c"}],` +
		`"cc_addresses":[{"name":"C","email":"c@b.c"}],"bcc_addresses":[{"name":"B","email":"b@b.c"}],` +
		`"date":"2026-10-31T10:00:00Z","body_text":"x","body_html":"<p>x</p>","is_read":true,"is_starred":true,` +
		`"has_attachments":true,"attachments":[{"id":4,"filename":"f","mime_type":"text/plain","size":5}],` +
		`"in_reply_to":"<r>","references":"<r>","category":"known","bulk_reason":"list-id",` +
		`"message_count":2,"unread_count":1,"email_ids":[1,9]}`
	api, _ := recording(t, full)
	email, err := api.Email(context.Background(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	json.Unmarshal([]byte(full), &want)
	encoded, _ := json.Marshal(email)
	json.Unmarshal(encoded, &got)
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("--json would drop %q", key)
		}
	}
	if len(email.BccAddresses) != 1 || email.BccAddresses[0].Email != "b@b.c" {
		t.Errorf("bcc decoded as %+v", email.BccAddresses)
	}
}

func TestSendCarriesBccInJSONAndInMultipart(t *testing.T) {
	api, seen := recording(t, `{"sent":true}`)
	ctx := context.Background()
	req := SendRequest{To: []string{"a@b.c"}, Bcc: []string{"x@b.c", "y@b.c"}, Body: "hi"}
	if err := api.Send(ctx, 1, req); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal((*seen)[0].body, &body)
	if bcc, _ := body["bcc"].([]any); len(bcc) != 2 {
		t.Errorf("JSON bcc = %v", body["bcc"])
	}
	file := filepath.Join(t.TempDir(), "n.txt")
	os.WriteFile(file, []byte("data"), 0o600)
	req.Attachments = []string{file}
	if err := api.Send(ctx, 1, req); err != nil {
		t.Fatal(err)
	}
	fields, files := formOf(t, (*seen)[1])
	if fields["bcc"] != "x@b.c,y@b.c" || len(files) != 1 {
		t.Errorf("multipart fields %v files %v", fields, files)
	}
}

func TestDraftWritesUseTheDraftRoutes(t *testing.T) {
	api, seen := recording(t, `{"id":7}`)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "n.txt")
	os.WriteFile(file, []byte("data"), 0o600)

	if id, err := api.SaveDraft(ctx, 1, SendRequest{To: []string{"a@b.c"}, Attachments: []string{file}}); err != nil || id != 7 {
		t.Fatalf("SaveDraft = %d, %v", id, err)
	}
	if id, err := api.UpdateDraft(ctx, 1, 7, SendRequest{Subject: "s", Bcc: []string{"x@b.c"}}, []int64{4}); err != nil || id != 7 {
		t.Fatalf("UpdateDraft = %d, %v", id, err)
	}
	if _, err := api.UpdateDraft(ctx, 1, 7, SendRequest{Attachments: []string{file}}, []int64{4, 5}); err != nil {
		t.Fatal(err)
	}
	got := *seen
	if got[0].method != "POST" || !strings.HasPrefix(got[0].ctype, "multipart/") {
		t.Errorf("create with a file: %s %s", got[0].method, got[0].ctype)
	}
	var body map[string]any
	json.Unmarshal(got[1].body, &body)
	if got[1].method != "PUT" || got[1].uri != "/api/accounts/1/mail/drafts/7" || body["subject"] != "s" {
		t.Errorf("update: %s %s %v", got[1].method, got[1].uri, body)
	}
	if drop, _ := body["remove_attachments"].([]any); len(drop) != 1 || body["bcc"] == nil {
		t.Errorf("update body %v", body)
	}
	fields, files := formOf(t, got[2])
	if got[2].method != "PUT" || fields["remove_attachments"] != "4,5" || len(files) != 1 {
		t.Errorf("multipart update: %s %v %v", got[2].method, fields, files)
	}
}

func TestMoveAndFolderRoutes(t *testing.T) {
	api, seen := recording(t, `{"ok":true,"id":9,"path":"Work","name":"Work","type":"custom","deleted":true}`)
	ctx := context.Background()
	api.MoveEmails(ctx, 1, []int64{3, 4}, MoveTarget{FolderType: "archive"})
	api.MoveEmails(ctx, 1, []int64{3}, MoveTarget{FolderID: 12})
	folder, err := api.CreateFolder(ctx, 1, "Work")
	if err != nil || folder.ID != 9 {
		t.Fatalf("CreateFolder = %+v, %v", folder, err)
	}
	api.RenameFolder(ctx, 1, 9, "Clients")
	api.DeleteFolder(ctx, 1, 9, false)
	api.DeleteFolder(ctx, 1, 9, true)

	want := []string{
		`POST /api/accounts/1/mail/emails/move {"email_ids":[3,4],"folder_type":"archive"}`,
		`POST /api/accounts/1/mail/emails/move {"email_ids":[3],"folder_id":12}`,
		`POST /api/accounts/1/mail/folders {"name":"Work"}`,
		`PATCH /api/accounts/1/mail/folders/9 {"name":"Clients"}`,
		`DELETE /api/accounts/1/mail/folders/9 `,
		`DELETE /api/accounts/1/mail/folders/9?force=true `,
	}
	for i, req := range *seen {
		if got := req.method + " " + req.uri + " " + string(req.body); got != want[i] {
			t.Errorf("request %d = %s, want %s", i, got, want[i])
		}
	}
}

func TestMoveRefusesBadBatchesBeforeTheNetwork(t *testing.T) {
	api, seen := recording(t, `{}`)
	if err := api.MoveEmails(context.Background(), 1, nil, MoveTarget{FolderType: "trash"}); err == nil {
		t.Error("an empty move must be refused")
	}
	if err := api.MoveEmails(context.Background(), 1, make([]int64, 201), MoveTarget{FolderType: "trash"}); err == nil {
		t.Error("201 ids must be refused")
	}
	if len(*seen) != 0 {
		t.Errorf("%d requests reached the server", len(*seen))
	}
}
