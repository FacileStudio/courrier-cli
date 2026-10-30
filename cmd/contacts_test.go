package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func accountHonouringInstance(t *testing.T, seen *[]string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/mail/contacts" {
			*seen = append(*seen, r.URL.Query().Get("account"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"contacts":[],"total":0,"page":1,"limit":50}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("COURRIER_SERVER_URL", server.URL)
	t.Setenv("COURRIER_TOKEN", "tok")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	flagJSON = true
	t.Cleanup(func() { flagJSON, flagAccount, flagContactsAll = false, 0, false })
}

func TestContactsSendsTheAccountTheServerHonours(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		flag  int64
		all   bool
		wants string
	}{
		{name: "account flag", env: "", flag: 5, wants: "5"},
		{name: "default account", env: "7", wants: "7"},
		{name: "all sends nothing", env: "7", all: true, wants: ""},
		{name: "no default sends nothing", wants: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			accountHonouringInstance(t, &seen)
			t.Setenv("COURRIER_ACCOUNT", tt.env)
			flagAccount, flagContactsAll = tt.flag, tt.all

			if _, err := captureStdout(t, runContacts); err != nil {
				t.Fatalf("runContacts: %v", err)
			}
			if len(seen) != 1 || seen[0] != tt.wants {
				t.Errorf("server saw account %q, want %q", seen, tt.wants)
			}
		})
	}
}
