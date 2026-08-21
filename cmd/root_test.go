package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
)

// accountsServer answers the accounts route with the ids given, so
// resolveAccount can be exercised without an instance.
func accountsServer(t *testing.T, body string) *client.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return client.New(server.URL, "token")
}

// TestResolveAccountPrecedence pins the order the CLI standard requires: the
// flag beats the environment, which beats the stored default. Getting this
// wrong sends mail from the wrong address, which no retry undoes.
func TestResolveAccountPrecedence(t *testing.T) {
	api := accountsServer(t, `{"accounts":[{"id":7},{"id":9}]}`)
	stored := config.Config{DefaultAccount: 3}

	t.Run("flag wins", func(t *testing.T) {
		flagAccount = 1
		t.Cleanup(func() { flagAccount = 0 })
		t.Setenv("COURRIER_ACCOUNT", "2")

		got, err := resolveAccount(context.Background(), api, stored)
		if err != nil || got != 1 {
			t.Fatalf("got (%d, %v), want (1, nil)", got, err)
		}
	})

	t.Run("environment beats the stored default", func(t *testing.T) {
		t.Setenv("COURRIER_ACCOUNT", "2")

		got, err := resolveAccount(context.Background(), api, stored)
		if err != nil || got != 2 {
			t.Fatalf("got (%d, %v), want (2, nil)", got, err)
		}
	})

	t.Run("stored default is used when nothing overrides it", func(t *testing.T) {
		t.Setenv("COURRIER_ACCOUNT", "")

		got, err := resolveAccount(context.Background(), api, stored)
		if err != nil || got != 3 {
			t.Fatalf("got (%d, %v), want (3, nil)", got, err)
		}
	})
}

// TestResolveAccountFallsBackToTheOnlyAccount covers the case login records for
// the user, and the two it must refuse rather than guess.
func TestResolveAccountFallsBackToTheOnlyAccount(t *testing.T) {
	t.Setenv("COURRIER_ACCOUNT", "")
	empty := config.Config{}

	t.Run("exactly one account needs no flag", func(t *testing.T) {
		got, err := resolveAccount(context.Background(), accountsServer(t, `{"accounts":[{"id":11}]}`), empty)
		if err != nil || got != 11 {
			t.Fatalf("got (%d, %v), want (11, nil)", got, err)
		}
	})

	t.Run("several accounts refuse to be guessed at", func(t *testing.T) {
		_, err := resolveAccount(context.Background(), accountsServer(t, `{"accounts":[{"id":11},{"id":12}]}`), empty)
		if err == nil {
			t.Fatal("want an error naming --account, got nil")
		}
	})

	t.Run("no accounts says so", func(t *testing.T) {
		_, err := resolveAccount(context.Background(), accountsServer(t, `{"accounts":[]}`), empty)
		if err == nil {
			t.Fatal("want an error, got nil")
		}
	})
}

// TestFolderTypesAreTheRouteSegments guards the set the listing route addresses
// folders by. A folder id typed here would otherwise reach the instance and
// come back as an unexplained 404.
func TestFolderTypesAreTheRouteSegments(t *testing.T) {
	want := map[string]bool{
		"inbox": true, "sent": true, "drafts": true,
		"trash": true, "junk": true, "archive": true, "custom": true,
	}
	if len(folderTypes) != len(want) {
		t.Fatalf("folderTypes has %d entries, want %d", len(folderTypes), len(want))
	}
	for _, folderType := range folderTypes {
		if !want[folderType] {
			t.Errorf("unexpected folder type %q", folderType)
		}
	}
}
