// Package loopback implements the listener half of the porte SSO CLI flow
// described in CLI-STANDARD §6.4. It binds an ephemeral loopback port, builds
// the URL the browser starts the login at, and waits for porte to redirect back
// with a one-time code. Trading that code for a session token is the client
// package's job, not this one's.
package loopback

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// waitTimeout bounds a single login attempt. It is long enough for a first-time
// browser sign-in with a password manager and an MFA prompt, and short enough
// that an abandoned terminal does not sit on an open port forever.
const waitTimeout = 3 * time.Minute

// shutdownGrace lets the success page finish reaching the browser after the
// code has been read, so the user sees the page instead of a reset connection.
const shutdownGrace = 2 * time.Second

// successPage is what the browser shows once the code has landed. It exists to
// send the user back to the terminal, so it stays plain and carries no assets
// that a strict browser would have to fetch from somewhere else.
const successPage = `<!doctype html>
<meta charset="utf-8">
<title>Signed in</title>
<body style="font-family: system-ui, sans-serif; margin: 4rem auto; max-width: 28rem; line-height: 1.6">
<h1 style="font-size: 1.25rem">Signed in</h1>
<p>Courrier has your login. You can close this tab and go back to the terminal.</p>
</body>
`

// Listener is a bound loopback socket waiting for the login redirect. The port
// is taken before the login URL is built, so the URL can name a port that is
// already listening and nothing else can claim it in between.
type Listener struct {
	listener net.Listener
	port     int
}

// Listen binds 127.0.0.1 on an ephemeral port. The port is chosen by the kernel
// rather than fixed, so two shells can run a login at the same time without
// coordinating. The caller must Close the listener.
func Listen() (*Listener, error) {
	socket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("cannot open the login listener — %w", err)
	}
	return &Listener{listener: socket, port: socket.Addr().(*net.TCPAddr).Port}, nil
}

// Port reports the loopback port the browser will be redirected back to.
func (l *Listener) Port() int {
	return l.port
}

// LoginURL builds the address the browser starts the flow at, against a base
// such as https://courrier.facile.studio. Courrier mounts porte under /api, and
// porte reads exactly the parameters flow, port and cli_state: rename one and
// the browser completes a normal web login instead of the CLI one, leaving this
// listener waiting for a redirect that never comes.
func (l *Listener) LoginURL(base, state string) string {
	query := url.Values{}
	query.Set("flow", "cli")
	query.Set("port", strconv.Itoa(l.port))
	query.Set("cli_state", state)
	return strings.TrimRight(base, "/") + "/api/auth/oidc?" + query.Encode()
}

// RandomState returns a fresh nonce for one login. porte echoes it back on the
// redirect, which is what proves the callback belongs to this login rather than
// to any page that guessed the port. Sixteen random bytes are encoded
// base64url without padding, which is 22 characters drawn from [A-Za-z0-9-_] —
// inside porte's 128-character bound and its permitted alphabet.
func RandomState() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("cannot draw a login nonce — %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(nonce), nil
}

// OpenBrowser hands the URL to the operating system's default browser. It is
// best effort: false means no browser could be launched and the caller should
// print the URL for the user to open by hand.
func OpenBrowser(rawURL string) bool {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", rawURL)
	case "windows":
		command = exec.Command("cmd", "/C", "start", "", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}
	return command.Start() == nil
}

// WaitForCode serves the login callback and returns the one-time code porte
// redirected back with. It gives up when ctx is cancelled — Ctrl-C — or after
// waitTimeout, whichever comes first.
//
// Only a request to / that carries a code and echoes state back is accepted.
// Anything else is answered and ignored, and the listener keeps waiting: a
// browser asks for /favicon.ico without being told to, and ending the login
// over that produces a failure the user cannot diagnose. Refusing a mismatched
// state is the other half of the same rule — without it, any page the user has
// open can hit http://127.0.0.1:<port>/?code=... and hand this CLI a session
// that is not the user's.
func (l *Listener) WaitForCode(ctx context.Context, state string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	codes := make(chan string, 1)
	failures := make(chan error, 1)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "This callback does not belong to the login that is waiting.", http.StatusBadRequest)
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "This callback carries no login code.", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(successPage))
		select {
		case codes <- code:
		default:
		}
	})}
	server.SetKeepAlivesEnabled(false)

	go func() {
		if err := server.Serve(l.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- err
		}
	}()

	defer func() {
		grace, stop := context.WithTimeout(context.Background(), shutdownGrace)
		defer stop()
		_ = server.Shutdown(grace)
	}()

	select {
	case code := <-codes:
		return code, nil
	case err := <-failures:
		return "", fmt.Errorf("the login listener failed — %w", err)
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("gave up after %s waiting for the browser to finish the login", waitTimeout)
		}
		return "", ctx.Err()
	}
}

// Close releases the loopback port. It is safe to call after WaitForCode, which
// closes the socket itself as it shuts the callback server down.
func (l *Listener) Close() error {
	if err := l.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
