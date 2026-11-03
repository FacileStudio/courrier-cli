package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/FacileStudio/porte/devicelogin"
	"github.com/FacileStudio/porte/loopback"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// porteMount is the path Courrier's API mounts porte under, and the reason the
// login URL is /api/auth/oidc rather than /auth/oidc. porte's routes are
// relative to whatever router carries them, so where they hang is Courrier's
// decision and not porte's.
//
// A login URL built against the wrong mount is not an error anywhere. The
// browser completes an ordinary web login, the user lands on the dashboard, and
// the listener sits out its three minutes waiting for a redirect nobody sends.
const porteMount = "/api"

// loginAppName is the tool the loopback page names. By the time the browser is
// on 127.0.0.1 it has left Courrier's domain behind and the address bar proves
// nothing, so the name on the page is all a user has to tell this login from
// any other local process that asked them to sign in.
const loginAppName = "Courrier"

// loginViaSSO runs the device grant through porte/devicelogin and returns the
// session token. An instance that has not shipped the device exchange, or
// --listener, gets loginViaListener instead.
func loginViaSSO(ctx context.Context, api *client.Client, base string) (string, error) {
	if loginListener {
		return loginViaListener(ctx, api, base)
	}
	s, err := devicelogin.Login(ctx, devicelogin.Options{
		AppOrigin:   base,
		Mount:       porteMount,
		OpenBrowser: func(link string) bool { return !loginNoBrowser && loopback.OpenBrowser(link) },
	})
	switch {
	case errors.Is(err, devicelogin.ErrExchangeNotMounted), errors.Is(err, devicelogin.ErrNoDeviceGrant):
		ui.Warn("this server has no device login yet, using the browser listener")
		return loginViaListener(ctx, api, base)
	case errors.Is(err, devicelogin.ErrAccountNotLinked):
		return "", fmt.Errorf("this account has not signed in to %s yet — sign in once at %s in a browser, then run login again", base, base)
	case err != nil:
		return "", wrapInterrupt(ctx, err)
	}
	return s.Token, nil
}

// loginViaListener runs the porte loopback flow: a listener on loopback, a
// browser, a redirect back carrying a one-time code, and an exchange for the
// session token. The state nonce is minted here and verified by the listener, so a
// callback belonging to another login is refused.
//
// The listener is porte/loopback rather than a copy of it. Three CLIs each grew
// their own, and the parts that matter are the parts that are silent when they
// are wrong: a callback whose state does not match must be refused without
// ending the login, because ending it lets any page the user has open close a
// login it did not start, and accepting it lets that page hand this CLI a
// session that is not the user's. One reviewed copy is the only way that stays
// true in all three.
func loginViaListener(ctx context.Context, api *client.Client, base string) (string, error) {
	listener, err := loopback.Listen()
	if err != nil {
		return "", err
	}
	defer listener.Close()
	listener.AppName = loginAppName

	state, err := loopback.RandomState()
	if err != nil {
		return "", err
	}

	target := listener.LoginURL(base, porteMount, state)
	if loginNoBrowser || !loopback.OpenBrowser(target) {
		loginShowURL(target)
	} else {
		loginStep("opening your browser to sign in")
		loginHint("if nothing opened, run login again with --no-browser and open the URL yourself")
	}

	loginStep("waiting for the login to complete")
	code, err := listener.WaitForCode(ctx, state)
	if err != nil {
		return "", wrapInterrupt(ctx, err)
	}

	token, err := api.Exchange(ctx, code)
	if err != nil {
		return "", wrapInterrupt(ctx, fmt.Errorf("the login code was refused — run login again, a code is good for sixty seconds and works once: %w", err))
	}
	return token, nil
}
