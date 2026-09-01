package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	flagKeysListApp       string
	flagKeysCreateApp     string
	flagKeysCreatePublic  bool
	flagKeysCreateOrigins string
	flagKeysCreateQuota   int
	flagKeysRevokeYes     bool
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage API keys",
	Long: `Manage API keys for applications and automated services.

List existing keys, generate new secret or public keys, and revoke keys
that are no longer needed.`,
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	Long: `List the registered API keys.

Filter by application name with --app <name>.`,
	Args: cobra.NoArgs,
	RunE: runKeysList,
}

var keysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	Long: `Create a new API key for an application.

Secret keys are intended for backend services. Public keys are intended for
browser applications and require allowed origins.

The raw token is printed on stdout once and cannot be retrieved later.`,
	Args: cobra.NoArgs,
	RunE: runKeysCreate,
}

var keysRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an API key",
	Long: `Revoke an API key by its numeric id.

Revocation is immediate and cannot be undone. Confirmation is requested when
stdin is a terminal; pass --yes to skip the confirmation.`,
	Args: cobra.ExactArgs(1),
	RunE: runKeysRevoke,
}

func init() {
	keysCmd.AddCommand(keysListCmd, keysCreateCmd, keysRevokeCmd)

	keysListCmd.Flags().StringVar(&flagKeysListApp, "app", "", "Filter keys by application name")

	keysCreateCmd.Flags().StringVar(&flagKeysCreateApp, "app", "", "Application name for the key")
	keysCreateCmd.Flags().BoolVar(
		&flagKeysCreatePublic, "public", false, "Create a public browser key instead of a secret key",
	)
	keysCreateCmd.Flags().StringVar(
		&flagKeysCreateOrigins, "origins", "", "Allowed origins for public keys, comma-separated",
	)
	keysCreateCmd.Flags().IntVar(&flagKeysCreateQuota, "quota", 0, "Daily request quota limit, 0 for unlimited")
	_ = keysCreateCmd.MarkFlagRequired("app")

	keysRevokeCmd.Flags().BoolVar(&flagKeysRevokeYes, "yes", false, "Answer the confirmation with yes")
}

func runKeysList(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	api, _, err := session()
	if err != nil {
		return err
	}

	keys, err := api.ListKeys(ctx, flagKeysListApp)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}

	if flagJSON {
		return ui.JSON(keys)
	}

	renderKeys(keys)
	return nil
}

func runKeysCreate(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	if strings.TrimSpace(flagKeysCreateApp) == "" {
		return errors.New("app name is required")
	}
	if flagKeysCreateQuota < 0 {
		return errors.New("quota cannot be negative")
	}

	api, _, err := session()
	if err != nil {
		return err
	}

	kind := "secret"
	if flagKeysCreatePublic {
		kind = "public"
	}

	req := client.CreateKeyRequest{
		App:            flagKeysCreateApp,
		Kind:           kind,
		AllowedOrigins: parseOrigins(flagKeysCreateOrigins),
		DailyQuota:     flagKeysCreateQuota,
	}

	res, err := api.CreateKey(ctx, req)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}

	if flagJSON {
		return ui.JSON(res)
	}

	ui.Plain("%s", res.Token)
	return nil
}

func runKeysRevoke(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("%q is not a valid key id", args[0])
	}

	api, _, err := session()
	if err != nil {
		return err
	}

	if err := confirmRevoke(id); err != nil {
		return err
	}

	if err := api.RevokeKey(ctx, id); err != nil {
		return wrapInterrupt(ctx, err)
	}

	if flagJSON {
		return ui.JSON(map[string]any{
			"id":      id,
			"revoked": true,
		})
	}

	ui.Success("API key %d revoked", id)
	return nil
}

func confirmRevoke(id int64) error {
	if flagKeysRevokeYes {
		return nil
	}
	if flagJSON {
		return errors.New("revoke with --json needs --yes, structured output never prompts")
	}
	if !stdinIsTerminal() {
		return errors.New("revoke needs --yes when stdin is not a terminal, nothing there can answer the confirmation")
	}

	ui.Warn("about to revoke API key %d", id)
	confirmed, err := confirmAction("revoke it?")
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("revocation cancelled, nothing was changed")
	}
	return nil
}

func parseOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}
