package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

const foldersJSON = `{"folders":[{"id":1,"path":"INBOX","name":"INBOX","type":"inbox"},` +
	`{"id":2,"path":"Drafts","name":"Drafts","type":"drafts"},` +
	`{"id":3,"path":"Clients","name":"Clients","type":"custom"},` +
	`{"id":4,"path":"A/Dup","name":"Dup","type":"custom"},{"id":5,"path":"B/Dup","name":"Dup","type":"custom"}]}`

const draftJSON = `{"id":2,"folder_id":2,"subject":"old","body_text":"old body",` +
	`"to_addresses":[{"email":"to@x.test"}],"cc_addresses":[],"bcc_addresses":[{"email":"hidden@x.test"}]}`

func mailInstance(t *testing.T, answers map[string]string) *[]string {
	t.Helper()
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		key := r.Method + " " + r.URL.Path
		seen = append(seen, key+" "+string(raw))
		answer, ok := answers[key]
		if !ok {
			answer = `{"ok":true}`
		}
		status := http.StatusOK
		if strings.HasPrefix(answer, "400:") {
			status, answer = http.StatusBadRequest, strings.TrimPrefix(answer, "400:")
		}
		w.WriteHeader(status)
		io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	t.Setenv("COURRIER_SERVER_URL", server.URL)
	t.Setenv("COURRIER_TOKEN", "tok")
	t.Setenv("COURRIER_ACCOUNT", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })
	return &seen
}

func TestFindFolderByIDNameAndPath(t *testing.T) {
	var out struct{ Folders []client.Folder }
	json.Unmarshal([]byte(foldersJSON), &out)
	for ref, want := range map[string]int64{"3": 3, "clients": 3, "CLIENTS": 3, "B/Dup": 5, "inbox": 1} {
		got, err := findFolder(out.Folders, ref)
		if err != nil || got.ID != want {
			t.Errorf("findFolder(%q) = %d, %v; want %d", ref, got.ID, err, want)
		}
	}
	for _, ref := range []string{"Dup", "missing", "99"} {
		if _, err := findFolder(out.Folders, ref); err == nil {
			t.Errorf("findFolder(%q) must fail", ref)
		}
	}
}

func TestMoveResolvesATypeAnIDAndAName(t *testing.T) {
	seen := mailInstance(t, map[string]string{"GET /api/accounts/1/mail/folders": foldersJSON})
	t.Cleanup(func() { flagMoveTo = "" })
	for _, to := range []string{"archive", "3", "clients"} {
		flagMoveTo = to
		if _, err := captureStdout(t, func() error { return runMove(moveCmd, []string{"10", "11"}) }); err != nil {
			t.Fatalf("--to %s: %v", to, err)
		}
	}
	want := []string{
		`POST /api/accounts/1/mail/emails/move {"email_ids":[10,11],"folder_type":"archive"}`,
		`POST /api/accounts/1/mail/emails/move {"email_ids":[10,11],"folder_id":3}`,
		`GET /api/accounts/1/mail/folders `,
		`POST /api/accounts/1/mail/emails/move {"email_ids":[10,11],"folder_id":3}`,
	}
	if strings.Join(*seen, "|") != strings.Join(want, "|") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(*seen, "\n"), strings.Join(want, "\n"))
	}
}

func TestMoveRefusesCustomAsATypeAndBadIDs(t *testing.T) {
	seen := mailInstance(t, nil)
	t.Cleanup(func() { flagMoveTo = "" })
	flagMoveTo = "custom"
	if err := runMove(moveCmd, []string{"1"}); err == nil {
		t.Error("--to custom must be refused")
	}
	flagMoveTo = "trash"
	if err := runMove(moveCmd, []string{"abc"}); err == nil {
		t.Error("a bad id must be refused")
	}
	if len(*seen) != 0 {
		t.Errorf("requests reached the server: %v", *seen)
	}
}

func TestMarkStarUsesOneBulkCall(t *testing.T) {
	seen := mailInstance(t, nil)
	flagMarkStar = true
	t.Cleanup(func() { flagMarkStar = false })
	if _, err := captureStdout(t, func() error { return runMark(markCmd, []string{"5", "6", "7"}) }); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], `"action":"star"`) {
		t.Errorf("requests: %v", *seen)
	}
}

func TestMarkStarFallsBackOnAnInstanceWithoutTheBulkAction(t *testing.T) {
	seen := mailInstance(t, map[string]string{
		"POST /api/accounts/1/mail/emails/bulk-action": `400:{"error":{"code":"invalid_argument","message":"unknown action"}}`,
	})
	flagMarkUnstar = true
	t.Cleanup(func() { flagMarkUnstar = false })
	if _, err := captureStdout(t, func() error { return runMark(markCmd, []string{"5", "6"}) }); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 3 || !strings.HasPrefix((*seen)[1], "PATCH /api/accounts/1/mail/emails/5") {
		t.Errorf("requests: %v", *seen)
	}
}

func TestDraftUpdateKeepsTheIDAndTheStoredFields(t *testing.T) {
	seen := mailInstance(t, map[string]string{
		"GET /api/accounts/1/mail/emails/2": draftJSON,
		"GET /api/accounts/1/mail/folders":  foldersJSON,
		"PUT /api/accounts/1/mail/drafts/2": `{"id":2}`,
	})
	if err := draftUpdateCmd.Flags().Set("subject", "new"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { draftUpdateCmd.Flags().Lookup("subject").Changed = false; draftEditFlag = composeFlags{} })
	out, err := captureStdout(t, func() error { return runDraftUpdate(draftUpdateCmd, []string{"2"}) })
	if err != nil {
		t.Fatal(err)
	}
	var put string
	for _, line := range *seen {
		if strings.HasPrefix(line, "PUT ") {
			put = line
		}
		if strings.HasPrefix(line, "POST ") || strings.HasPrefix(line, "DELETE ") {
			t.Errorf("update must not create or delete: %s", line)
		}
	}
	for _, part := range []string{`"subject":"new"`, `"bcc":["hidden@x.test"]`, `"body":"old body"`, `"to":["to@x.test"]`} {
		if !strings.Contains(put, part) {
			t.Errorf("PUT body lacks %s: %s", part, put)
		}
	}
	if !strings.Contains(out, `"id": 2`) {
		t.Errorf("report: %s", out)
	}
}

func TestFolderWritesRefuseSystemFoldersBeforeTheRequest(t *testing.T) {
	seen := mailInstance(t, map[string]string{"GET /api/accounts/1/mail/folders": foldersJSON})
	flagFolderYes = true
	t.Cleanup(func() { flagFolderYes = false })
	if err := runFolderDelete(foldersDeleteCmd, []string{"inbox"}); err == nil {
		t.Error("deleting the inbox must be refused")
	}
	if err := runFolderRename(foldersRenameCmd, []string{"1", "Mail"}); err == nil {
		t.Error("renaming the inbox must be refused")
	}
	for _, line := range *seen {
		if !strings.HasPrefix(line, "GET ") {
			t.Errorf("a write reached the server: %s", line)
		}
	}
}

func TestSendAcceptsABccOnlyRecipientList(t *testing.T) {
	flagSendTo, flagSendCc, flagSendBcc = nil, nil, []string{"b@x.test"}
	t.Cleanup(func() { flagSendBcc = nil })
	if err := validateRecipients(); err != nil {
		t.Errorf("bcc-only: %v", err)
	}
	flagSendBcc = []string{"nope"}
	if err := validateRecipients(); err == nil {
		t.Error("a bad bcc address must be refused")
	}
	flagSendBcc = nil
	if err := validateRecipients(); err == nil {
		t.Error("no recipient at all must be refused")
	}
	_ = os.Stdout
}
