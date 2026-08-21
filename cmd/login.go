package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
	"github.com/FacileStudio/courrier-cli/internal/loopback"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	loginEmail     string
	loginPassword  string
	loginNoBrowser bool
)

// errSSOOnly is what an instance running SSO_ONLY looks like from here: porte
// does not register the local credential routes at all, so the password login
// is not a broken endpoint but an absent one, and no retry or different
// password will find it.
var errSSOOnly = errors.New("this instance has no password login — it runs single sign-on only, so sign in on a machine with a browser or set COURRIER_TOKEN to a dashboard token")

// loginCmd stores the instance URL, the session token it returns and the
// mailbox to default to, so no later command needs any of the three.
//
// The stored configuration is loaded and only the fields this command owns are
// replaced, so a key written by a future version survives a login instead of
// being reset to its zero value.
var loginCmd = &cobra.Command{
	Use:   "login [url]",
	Short: "Authenticate with a Courrier instance",
	Long: `Stores the instance URL and the session token it returns, so later commands
need neither. The URL defaults to the one already stored.

The instance is asked what it accepts before anything is typed. Where it offers
single sign-on, a browser opens, the login redirects back to a one-time code on
this machine, and the code is traded for a session token. An instance without
single sign-on takes an address and a password instead.

A login that finds exactly one mail account records it as the default.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signalContext()
		defer stop()

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.URL = resolveLoginURL(cfg.URL, args)

		api := client.New(cfg.URL, "")
		auth, err := api.AuthConfig(ctx)
		if err != nil {
			return wrapInterrupt(ctx, err)
		}

		var token string
		switch {
		case auth.OIDCEnabled:
			token, err = loginViaSSO(ctx, api, cfg.URL)
		case auth.SSOOnly:
			err = errSSOOnly
		default:
			loginWarnNoSSO()
			token, err = loginViaPassword(ctx, api)
		}
		if err != nil {
			return err
		}

		cfg.Token = token
		cfg.DefaultAccount = resolveDefaultAccount(ctx, client.New(cfg.URL, token), cfg.DefaultAccount)
		if err := config.Save(cfg); err != nil {
			return err
		}

		if flagJSON {
			return ui.JSON(struct {
				URL     string `json:"url"`
				Account int64  `json:"account"`
			}{cfg.URL, cfg.DefaultAccount})
		}
		ui.Success("signed in to %s", cfg.URL)
		return nil
	},
}

// resolveLoginURL applies the precedence the CLI standard requires, highest
// first: the positional argument, --url, COURRIER_SERVER_URL, the stored URL,
// then the built-in default.
func resolveLoginURL(stored string, args []string) string {
	resolved := stored
	if fromEnv := os.Getenv("COURRIER_SERVER_URL"); fromEnv != "" {
		resolved = config.NormalizeURL(fromEnv)
	}
	if flagURL != "" {
		resolved = config.NormalizeURL(flagURL)
	}
	if len(args) == 1 {
		resolved = config.NormalizeURL(args[0])
	}
	if resolved == "" {
		resolved = config.DefaultURL
	}
	return resolved
}

// loginViaSSO runs the porte CLI flow: a listener on loopback, a browser, a
// redirect back carrying a one-time code, and an exchange for the session
// token. The state nonce is minted here and verified by the listener, so a
// callback belonging to another login is refused.
func loginViaSSO(ctx context.Context, api *client.Client, base string) (string, error) {
	listener, err := loopback.Listen()
	if err != nil {
		return "", err
	}
	defer listener.Close()

	state, err := loopback.RandomState()
	if err != nil {
		return "", err
	}

	target := listener.LoginURL(base, state)
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

// loginViaPassword exchanges an address and a password for a session token,
// prompting only for what the flags did not supply.
//
// A 404 from the exchange is not a missing endpoint but an instance running
// SSO_ONLY, which registers no local credential routes at all — reporting it as
// "not found" would send the user hunting for a typo in a URL that is correct.
func loginViaPassword(ctx context.Context, api *client.Client) (string, error) {
	email, err := loginAskEmail()
	if err != nil {
		return "", err
	}
	password, err := loginAskPassword()
	if err != nil {
		return "", err
	}

	token, err := api.PasswordLogin(ctx, email, password)
	if err != nil {
		var apiErr *client.Error
		if errors.As(err, &apiErr) {
			switch {
			case apiErr.NotFound():
				return "", errSSOOnly
			case apiErr.Unauthenticated():
				return "", errors.New("the address or password was refused — check both and try again")
			}
		}
		return "", wrapInterrupt(ctx, fmt.Errorf("login failed — %w", err))
	}
	return token, nil
}

// loginAskEmail returns the address to log in with, prompting only when --email
// was not given and stdin is a terminal. A pipe or an agent harness gets an
// error naming the flag instead of a prompt nobody can answer, because a CLI
// blocked on a hidden prompt hangs its caller until the timeout.
func loginAskEmail() (string, error) {
	if loginEmail != "" {
		return loginEmail, nil
	}
	if !stdinIsTerminal() {
		return "", errors.New("no email address and stdin is not a terminal — pass --email")
	}

	fmt.Fprint(os.Stderr, "Email: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	email := strings.TrimSpace(line)
	if email == "" {
		return "", errors.New("an email address is required — pass --email")
	}
	return email, nil
}

// loginAskPassword returns the password, read without echo and followed by the
// newline the terminal did not print. As with the address, a non-terminal stdin
// is refused rather than prompted.
func loginAskPassword() (string, error) {
	if loginPassword != "" {
		return loginPassword, nil
	}
	if !stdinIsTerminal() {
		return "", errors.New("no password and stdin is not a terminal — pass --password, or set COURRIER_TOKEN to a dashboard token")
	}

	fmt.Fprint(os.Stderr, "Password: ")
	entered, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if len(entered) == 0 {
		return "", errors.New("a password is required — pass --password")
	}
	return string(entered), nil
}

// resolveDefaultAccount records which mailbox later commands act on when no
// --account is given, returning the default to store.
//
// A failure to list is warned about rather than returned: the session is
// already valid, and losing it over a follow-up call would make the user log in
// twice. A stored default that no longer names an account on this instance is
// dropped, since keeping it only produces a 404 from the next command.
func resolveDefaultAccount(ctx context.Context, api *client.Client, current int64) int64 {
	accounts, err := api.Accounts(ctx)
	if err != nil {
		ui.Warn("signed in, but the mail accounts could not be listed — %s", err)
		return current
	}

	switch len(accounts) {
	case 0:
		ui.Warn("this instance has no mail account yet — add one in the dashboard before reading mail")
		return 0
	case 1:
		loginStep("default mail account: %s (id %d)", accounts[0].Email, accounts[0].ID)
		return accounts[0].ID
	}

	if !flagJSON {
		renderAccounts(accounts, current)
		loginHint("pass --account <id> or set COURRIER_ACCOUNT to pick one")
	}
	if knownAccount(accounts, current) {
		return current
	}
	return 0
}

// knownAccount reports whether an account id still names one of the instance's
// mailboxes.
func knownAccount(accounts []client.Account, id int64) bool {
	for _, account := range accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

// loginWarnNoSSO says why a password is about to be asked for, on stderr where
// a warning belongs.
func loginWarnNoSSO() {
	ui.Warn("this instance has no single sign-on configured")
}

// loginShowURL prints the address the browser could not be sent to. Under
// --json it goes to stderr as a warning, because stdout carries the one
// document and nothing else.
func loginShowURL(target string) {
	if flagJSON {
		ui.Warn("open this URL to sign in — %s", target)
		return
	}
	ui.Step("open this URL to sign in")
	ui.Plain("%s", target)
}

// loginStep announces progress, unless --json is set and stdout is reserved for
// the single document.
func loginStep(format string, a ...any) {
	if !flagJSON {
		ui.Step(format, a...)
	}
}

// loginHint explains the line above it, under the same --json rule as
// loginStep.
func loginHint(format string, a ...any) {
	if !flagJSON {
		ui.Hint(format, a...)
	}
}

func init() {
	loginCmd.Flags().StringVar(&loginEmail, "email", "", "Email address, skipping the prompt")
	loginCmd.Flags().StringVar(&loginPassword, "password", "", "Password, skipping the prompt")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Print the login URL instead of opening a browser")
}
