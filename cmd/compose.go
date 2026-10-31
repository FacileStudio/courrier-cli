package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

// composeFlags holds the message fields shared by draft, reply and forward.
// send keeps its own package-level flags and reuses only the helpers below.
type composeFlags struct {
	to         []string
	cc         []string
	bcc        []string
	references []string
	attach     []string
	subject    string
	body       string
	bodyFile   string
	html       string
	htmlFile   string
	inReplyTo  string
	yes        bool
	usedStdin  bool
}

// bind registers the shared flags on cmd. Attachments and the confirmation are
// optional: drafts take attachments but send nothing, so they bind --attach
// alone through bindAttach.
func (f *composeFlags) bind(cmd *cobra.Command, attachments bool) {
	flags := cmd.Flags()
	flags.StringSliceVar(&f.to, "to", nil, "Recipient address, repeatable")
	flags.StringSliceVar(&f.cc, "cc", nil, "Carbon copy address, repeatable")
	flags.StringSliceVar(&f.bcc, "bcc", nil, "Blind carbon copy address, repeatable")
	flags.StringVar(&f.subject, "subject", "", "Subject line")
	flags.StringVar(&f.body, "body", "", "Message body as plain text")
	flags.StringVar(&f.bodyFile, "body-file", "", "Read the plain text body from this file, or - for stdin")
	flags.StringVar(&f.html, "html", "", "Message body as HTML")
	flags.StringVar(&f.htmlFile, "html-file", "", "Read the HTML body from this file, or - for stdin")
	flags.StringVar(&f.inReplyTo, "in-reply-to", "", "Message-ID this message replies to")
	flags.StringSliceVar(&f.references, "references", nil, "Message-ID in the thread, repeatable")
	if attachments {
		f.bindAttach(cmd)
		flags.BoolVar(&f.yes, "yes", false, "Answer the confirmation with yes")
	}
}

// bindAttach registers --attach, an array so that a comma in a filename is not
// split.
func (f *composeFlags) bindAttach(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&f.attach, "attach", nil, "Attach this local file, repeatable")
}

// bodies resolves the plain text and HTML bodies. A nil result means the flag
// was not given, which lets draft update tell "keep" from "set to empty".
func (f *composeFlags) bodies(cmd *cobra.Command) (text, html *string, err error) {
	if text, err = f.pick(cmd, "body", "body-file", f.body, f.bodyFile); err != nil {
		return nil, nil, err
	}
	html, err = f.pick(cmd, "html", "html-file", f.html, f.htmlFile)
	return text, html, err
}

// pick chooses between an inline flag and its file twin.
func (f *composeFlags) pick(cmd *cobra.Command, inline, file, inlineValue, path string) (*string, error) {
	hasInline, hasFile := cmd.Flags().Changed(inline), cmd.Flags().Changed(file)
	switch {
	case hasInline && hasFile:
		return nil, fmt.Errorf("--%s and --%s cannot be combined — pass one or the other", inline, file)
	case hasInline:
		return &inlineValue, nil
	case hasFile:
		return f.readSource(path)
	}
	return nil, nil
}

// readSource reads a body from a file, or from stdin once for a dash.
func (f *composeFlags) readSource(path string) (*string, error) {
	var raw []byte
	var err error
	if path == "-" {
		if f.usedStdin {
			return nil, errors.New("only one body can come from stdin — read the other from a file")
		}
		f.usedStdin = true
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the body — %w", err)
	}
	value := string(raw)
	return &value, nil
}

// stdinBody falls back to a piped stdin when no body flag was given, matching
// send. It returns nil when stdin is a terminal or already spent.
func (f *composeFlags) stdinBody(text, html *string) (*string, error) {
	if text != nil || html != nil || f.usedStdin || stdinIsTerminal() {
		return text, nil
	}
	return f.readSource("-")
}

// request assembles the outgoing message from resolved bodies.
func (f *composeFlags) request(text, html *string) client.SendRequest {
	req := client.SendRequest{
		To:          f.to,
		Cc:          f.cc,
		Bcc:         f.bcc,
		Subject:     f.subject,
		InReplyTo:   f.inReplyTo,
		References:  f.references,
		Attachments: f.attach,
	}
	if text != nil {
		req.Body = *text
	}
	if html != nil {
		req.BodyHTML = *html
	}
	return req
}

// checkAddresses refuses a value that cannot be an address before any network
// call.
func checkAddresses(lists ...[]string) error {
	for _, address := range slices.Concat(lists...) {
		if !strings.Contains(address, "@") {
			return fmt.Errorf("%q is not an email address — check the --to, --cc and --bcc values", address)
		}
	}
	return nil
}
