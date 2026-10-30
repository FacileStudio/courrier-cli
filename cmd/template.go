package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

var (
	templateFlags    composeFlags
	flagTemplateName string
	flagTemplateSpc  string
	flagTemplateYes  bool
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"templates"},
	Short:   "Manage reusable message templates",
	Long: `Create, list, change and delete message templates.

Templates belong to the user, not to a mail account, and the instance allows
fifty. A template holds a name, a subject and a plain text and HTML body.`,
}

var templateListCmd = &cobra.Command{
	Use: "list", Short: "List templates", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return runTemplateList() },
}

var templateShowCmd = &cobra.Command{
	Use: "show <id>", Short: "Show one template", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runTemplateShow(args[0]) },
}

var templateCreateCmd = &cobra.Command{
	Use: "create", Short: "Create a template", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error { return runTemplateWrite(cmd, "") },
}

var templateUpdateCmd = &cobra.Command{
	Use: "update <id>", Short: "Change a template, keeping the fields you do not pass", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runTemplateWrite(cmd, args[0]) },
}

var templateDeleteCmd = &cobra.Command{
	Use: "delete <id>", Short: "Delete a template", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return runTemplateDelete(args[0]) },
}

// init wires the template commands and flags.
func init() {
	for _, c := range []*cobra.Command{templateCreateCmd, templateUpdateCmd} {
		c.Flags().StringVar(&flagTemplateName, "name", "", "Template name")
		c.Flags().StringVar(&templateFlags.subject, "subject", "", "Subject line")
		c.Flags().StringVar(&templateFlags.body, "body", "", "Plain text body")
		c.Flags().StringVar(&templateFlags.bodyFile, "body-file", "", "Read the plain text body from this file, or - for stdin")
		c.Flags().StringVar(&templateFlags.html, "html", "", "HTML body")
		c.Flags().StringVar(&templateFlags.htmlFile, "html-file", "", "Read the HTML body from this file, or - for stdin")
	}
	templateCreateCmd.Flags().StringVar(&flagTemplateSpc, "space", "", "Scope the template to this space id")
	templateListCmd.Flags().StringVar(&flagTemplateSpc, "space", "", "Only templates of this space id")
	templateDeleteCmd.Flags().BoolVar(&flagTemplateYes, "yes", false, "Answer the confirmation with yes")
	templateCmd.AddCommand(templateListCmd, templateShowCmd, templateCreateCmd, templateUpdateCmd, templateDeleteCmd)
}

// templateSession opens a session; templates are not per account.
func templateSession() (*client.Client, error) {
	api, _, err := session()
	return api, err
}

// runTemplateList prints every template.
func runTemplateList() error {
	ctx, stop := signalContext()
	defer stop()

	api, err := templateSession()
	if err != nil {
		return err
	}
	templates, err := api.Templates(ctx, flagTemplateSpc)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if templates == nil {
		templates = []client.Template{}
	}
	if flagJSON {
		return ui.JSON(templates)
	}
	rows := make([][]string, 0, len(templates))
	for _, t := range templates {
		rows = append(rows, []string{strconv.FormatInt(t.ID, 10), truncate(t.Name, 32), truncate(t.Subject, 48), relativeTime(t.UpdatedAt)})
	}
	if len(rows) == 0 {
		ui.Plain("No templates")
		return nil
	}
	ui.Table([]string{"ID", "NAME", "SUBJECT", "UPDATED"}, rows)
	return nil
}

// findTemplate returns one template by id, since the API has no single-item read.
func findTemplate(ctx context.Context, api *client.Client, rawID string) (client.Template, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return client.Template{}, fmt.Errorf("%q is not a template id — see `courrier template list`", rawID)
	}
	templates, err := api.Templates(ctx, "")
	if err != nil {
		return client.Template{}, err
	}
	for _, t := range templates {
		if t.ID == id {
			return t, nil
		}
	}
	return client.Template{}, fmt.Errorf("no template %d — see `courrier template list`", id)
}

// runTemplateShow prints one template in full.
func runTemplateShow(rawID string) error {
	ctx, stop := signalContext()
	defer stop()

	api, err := templateSession()
	if err != nil {
		return err
	}
	tmpl, err := findTemplate(ctx, api, rawID)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if flagJSON {
		return ui.JSON(tmpl)
	}
	ui.Plain("%s", ui.Dim(fmt.Sprintf("template %d  ·  %s", tmpl.ID, tmpl.Name)))
	ui.Plain("subject %s", tmpl.Subject)
	ui.Plain("")
	ui.Plain("%s", tmpl.BodyText)
	return nil
}

// runTemplateWrite creates a template, or with an id merges the flags over the
// stored one and replaces it.
func runTemplateWrite(cmd *cobra.Command, rawID string) error {
	ctx, stop := signalContext()
	defer stop()

	text, htmlBody, err := templateFlags.bodies(cmd)
	if err != nil {
		return err
	}
	api, err := templateSession()
	if err != nil {
		return err
	}
	in := client.TemplateInput{Name: flagTemplateName, Subject: templateFlags.subject}
	if text != nil {
		in.BodyText = *text
	}
	if htmlBody != nil {
		in.BodyHTML = *htmlBody
	}
	if rawID == "" {
		return createTemplate(ctx, api, cmd, in)
	}
	return updateTemplate(ctx, api, cmd, rawID, in, text, htmlBody)
}

// createTemplate sends a new template, which needs a name.
func createTemplate(ctx context.Context, api *client.Client, cmd *cobra.Command, in client.TemplateInput) error {
	if in.Name == "" {
		commandStarted = false
		return fmt.Errorf("a template needs a name — pass --name")
	}
	if flagTemplateSpc != "" {
		in.SpaceID = &flagTemplateSpc
	}
	out, err := api.CreateTemplate(ctx, in)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot create the template — %w", err))
	}
	return reportTemplate(out, "template %d created", out.ID)
}

// updateTemplate keeps every field the caller did not pass, because the API
// replaces the subject and both bodies on every write.
func updateTemplate(ctx context.Context, api *client.Client, cmd *cobra.Command, rawID string, in client.TemplateInput, text, htmlBody *string) error {
	stored, err := findTemplate(ctx, api, rawID)
	if err != nil {
		return wrapInterrupt(ctx, err)
	}
	if !cmd.Flags().Changed("subject") {
		in.Subject = stored.Subject
	}
	if text == nil {
		in.BodyText = stored.BodyText
	}
	if htmlBody == nil {
		in.BodyHTML = stored.BodyHTML
	}
	out, err := api.UpdateTemplate(ctx, stored.ID, in)
	if err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot update template %d — %w", stored.ID, err))
	}
	return reportTemplate(out, "template %d updated", out.ID)
}

// reportTemplate prints the written template as JSON or one success line.
func reportTemplate(t client.Template, format string, a ...any) error {
	if flagJSON {
		return ui.JSON(t)
	}
	ui.Success(format, a...)
	return nil
}

// runTemplateDelete removes one template after the usual confirmation.
func runTemplateDelete(rawID string) error {
	ctx, stop := signalContext()
	defer stop()

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("%q is not a template id — see `courrier template list`", rawID)
	}
	if err := confirmDestructive("template "+rawID, flagTemplateYes); err != nil {
		return err
	}
	api, err := templateSession()
	if err != nil {
		return err
	}
	if err := api.DeleteTemplate(ctx, id); err != nil {
		return wrapInterrupt(ctx, fmt.Errorf("cannot delete template %d — %w", id, err))
	}
	if flagJSON {
		return ui.JSON(map[string]any{"id": id, "deleted": true})
	}
	ui.Success("template %d deleted", id)
	return nil
}
