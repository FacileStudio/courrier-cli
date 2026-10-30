package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
)

func TestParseClasses(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
		bad  bool
	}{
		{raw: "", want: nil},
		{raw: "known", want: []string{"known"}},
		{raw: " Known , bulk,bulk ", want: []string{"known", "bulk"}},
		{raw: "known,unknown_human,bulk", want: []string{"known", "unknown_human", "bulk"}},
		{raw: "spam", bad: true},
		{raw: "known,human", bad: true},
	}
	for _, tt := range tests {
		got, err := parseClasses(tt.raw)
		if tt.bad != (err != nil) || !reflect.DeepEqual(got, tt.want) && !tt.bad {
			t.Errorf("parseClasses(%q) = (%v, %v), want %v (bad=%v)", tt.raw, got, err, tt.want, tt.bad)
		}
	}
}

func TestScopeAccount(t *testing.T) {
	t.Setenv("COURRIER_ACCOUNT", "")
	t.Cleanup(func() { flagAccount = 0 })

	if got, err := scopeAccount(config.Config{}, false); got != 0 || err != nil {
		t.Errorf("no default: got (%d, %v), want all accounts", got, err)
	}
	if got, _ := scopeAccount(config.Config{DefaultAccount: 4}, false); got != 4 {
		t.Errorf("default set: got %d, want 4", got)
	}
	if got, _ := scopeAccount(config.Config{DefaultAccount: 4}, true); got != 0 {
		t.Errorf("default set with --all: got %d, want all accounts", got)
	}
	flagAccount = 2
	if got, _ := scopeAccount(config.Config{DefaultAccount: 4}, false); got != 2 {
		t.Errorf("--account: got %d, want 2", got)
	}
	if _, err := scopeAccount(config.Config{}, true); err == nil {
		t.Error("--all with --account must be refused")
	}
}

func triageInstance(t *testing.T, status int, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	t.Setenv("COURRIER_SERVER_URL", server.URL)
	t.Setenv("COURRIER_TOKEN", "tok")
	t.Setenv("COURRIER_ACCOUNT", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = writer
	runErr := run()
	os.Stdout = saved
	writer.Close()
	defer reader.Close()
	out, _ := io.ReadAll(reader)
	return string(out), runErr
}

func TestTriageJSONIsTheAPIBody(t *testing.T) {
	golden, err := os.ReadFile("testdata/triage.json")
	if err != nil {
		t.Fatal(err)
	}
	triageInstance(t, http.StatusOK, string(golden))
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdout(t, runTriage)
	if err != nil {
		t.Fatalf("runTriage: %v", err)
	}
	if out != string(golden) {
		t.Errorf("stdout differs from the golden document:\n%s", out)
	}
}

func TestTriageOnAnOldServerIsUnsupported(t *testing.T) {
	triageInstance(t, http.StatusNotFound, `{"error":{"code":"not_found","message":"no route"}}`)

	out, err := captureStdout(t, runTriage)
	if err != client.ErrUnsupported {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
}

func TestTriageFooterCarriesTheCounts(t *testing.T) {
	out := client.Triage{Total: 18, Page: 1, Counts: client.TriageCounts{Known: 2, UnknownHuman: 1, Bulk: 15},
		Items: make([]client.TriageItem, 1)}
	footer := triageFooter(out)
	for _, want := range []string{"18 conversations", "2 known", "1 unknown_human", "15 bulk", "--page 2"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer %q lacks %q", footer, want)
		}
	}
}
