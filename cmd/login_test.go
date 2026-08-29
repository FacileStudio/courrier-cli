package cmd

import (
	"net/url"
	"testing"

	"github.com/FacileStudio/porte/loopback"
)

// TestLoginURLCarriesCourriersMountPoint pins the one part of the SSO flow this
// repo still owns now that porte/loopback holds the rest. Courrier serves porte
// under /api, and porte's routes hang off whatever router carries them, so the
// mount is passed in rather than assumed.
//
// A wrong mount is not an error on any side. The browser completes an ordinary
// web login, the user lands on the dashboard, and the listener waits out its
// three minutes for a redirect nobody sends, which is a failure with nothing in
// it a user could act on.
func TestLoginURLCarriesCourriersMountPoint(t *testing.T) {
	listener, err := loopback.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	target, err := url.Parse(listener.LoginURL("https://courrier.facile.studio", porteMount, "nonce"))
	if err != nil {
		t.Fatalf("LoginURL did not produce a URL: %v", err)
	}
	if target.Path != "/api/auth/oidc" {
		t.Errorf("path = %q, want /api/auth/oidc", target.Path)
	}
	if target.Query().Get("flow") != "cli" {
		t.Errorf("flow = %q, want cli", target.Query().Get("flow"))
	}
}
