// Package cmd implements the courrier command tree.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/config"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var version = "dev"

var (
	flagURL     string
	flagJSON    bool
	flagNoColor bool
	flagAccount int64
)

var rootCmd = &cobra.Command{
	Use:   "courrier",
	Short: "Terminal client for a Courrier instance",
	Long: `Courrier is the suite's self-hosted email client: it connects to your own
IMAP and SMTP servers and serves them over a JSON API. This is its terminal
client. It lists, reads, searches and sends mail without opening the dashboard,
and emits JSON for anything that pipes into a tool.

Account credentials stay in the instance. This client never sees them.`,
	SilenceUsage:     true,
	SilenceErrors:    true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) { commandStarted = true },
}

// commandStarted is set once a command's own body begins. Cobra validates flags
// and arguments before that happens, so an error arriving while this is still
// false is a usage error rather than a failure of the work — and those exit 2.
var commandStarted bool

// init wires the version template and the global flags.
//
// The version template is not decoration: cobra's default is
// `<bin> version <v>`, which facile's installer cannot parse when it verifies
// what it just installed. Structured output forces colour off in the same
// place, because a caller piping JSON into jq must not receive escape codes.
func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&flagURL, "url", "", "Courrier instance URL, overriding the stored one")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Print one JSON document and nothing else")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().Int64Var(&flagAccount, "account", 0, "Mail account id, overriding the default")

	cobra.OnInitialize(func() {
		if flagNoColor || flagJSON {
			ui.DisableColor()
		}
	})
}

// ErrInterrupted marks a command stopped by a signal rather than a failure.
var ErrInterrupted = errors.New("interrupted")

// Execute runs the command tree and maps the outcome onto an exit code:
// 0 success, 1 error, 2 usage error, 130 on SIGINT.
//
// 130 is 128 plus SIGINT, which is what a shell and every `while` loop expect
// from a process the user stopped.
func Execute() {
	rootCmd.AddCommand(
		loginCmd,
		logoutCmd,
		accountsCmd,
		syncCmd,
		foldersCmd,
		inboxCmd,
		listCmd,
		readCmd,
		searchCmd,
		markCmd,
		sendCmd,
		keysCmd,
	)

	err := rootCmd.Execute()
	switch {
	case err == nil:
		return
	case !commandStarted:
		ui.Error("%s", err)
		ui.Hint("run `courrier <command> --help` for usage")
		os.Exit(2)
	case errors.Is(err, ErrInterrupted):
		os.Exit(130)
	default:
		ui.Error("%s", err)
		os.Exit(1)
	}
}

// signalContext cancels on SIGINT and SIGTERM, so a long sync stops cleanly
// and reports an interrupted exit instead of a failure.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

// connect builds a client from the stored configuration, applying the
// precedence the CLI standard requires: flag, then environment, then the
// config file, then the built-in default.
//
// An explicit token in the environment wins over the stored one. That is the
// headless and CI path, and how an agent holding its own credential avoids
// touching the config file at all.
func connect() (*client.Client, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cfg, err
	}

	if fromEnv := os.Getenv("COURRIER_SERVER_URL"); fromEnv != "" {
		cfg.URL = config.NormalizeURL(fromEnv)
	}
	if fromEnv := os.Getenv("COURRIER_URL"); fromEnv != "" {
		cfg.URL = config.NormalizeURL(fromEnv)
	}
	if flagURL != "" {
		cfg.URL = config.NormalizeURL(flagURL)
	}
	if fromEnv := os.Getenv("COURRIER_TOKEN"); fromEnv != "" {
		cfg.Token = fromEnv
	}
	if cfg.URL == "" {
		cfg.URL = config.DefaultURL
	}

	return client.New(cfg.URL, cfg.Token), cfg, nil
}

// session builds a client and refuses early when no credential is held, so a
// command reports the missing login rather than a bare 401 from the instance.
func session() (*client.Client, config.Config, error) {
	api, cfg, err := connect()
	if err != nil {
		return nil, cfg, err
	}
	if cfg.Token == "" {
		return nil, cfg, errors.New("not logged in — run `courrier login`")
	}
	return api, cfg, nil
}

// resolveAccount decides which mail account a command acts on: the --account
// flag, then the stored default, then the only account there is. It refuses to
// guess between several, because sending from the wrong address is not a
// mistake a retry undoes.
func resolveAccount(ctx context.Context, api *client.Client, cfg config.Config) (int64, error) {
	if flagAccount != 0 {
		return flagAccount, nil
	}
	if fromEnv := os.Getenv("COURRIER_ACCOUNT"); fromEnv != "" {
		parsed, err := strconv.ParseInt(fromEnv, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("COURRIER_ACCOUNT is not a number: %q", fromEnv)
		}
		return parsed, nil
	}
	if cfg.DefaultAccount != 0 {
		return cfg.DefaultAccount, nil
	}

	accounts, err := api.Accounts(ctx)
	if err != nil {
		return 0, err
	}
	switch len(accounts) {
	case 0:
		return 0, errors.New("no mail accounts on this instance — add one in the dashboard first")
	case 1:
		return accounts[0].ID, nil
	}
	return 0, errors.New("several mail accounts — pass --account <id>, see `courrier accounts`")
}

// stdinIsTerminal reports whether somebody is there to answer a prompt.
//
// Every interactive path checks it first. A CLI that blocks on a hidden prompt
// hangs an unattended caller — a script, a CI job, an agent — until its timeout,
// with no output saying what it is waiting for. The rule is to refuse and name
// the flag that skips the prompt instead.
func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
