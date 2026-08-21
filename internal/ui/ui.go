// Package ui implements CLI-STANDARD §7: one glyph per severity, colour only on
// a TTY, warnings and errors on stderr so a piped command emits only data.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
)

var (
	cyan   = color.New(color.FgCyan)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed)
	faint  = color.New(color.Faint)
)

// DisableColor implements --no-color. NO_COLOR and TTY detection are already
// handled by fatih/color at init time.
func DisableColor() {
	color.NoColor = true
}

// Step announces work about to happen, on stdout.
func Step(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "%s %s\n", cyan.Sprint("▸"), fmt.Sprintf(format, a...))
}

// Success reports work that completed, on stdout.
func Success(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "%s %s\n", green.Sprint("✓"), fmt.Sprintf(format, a...))
}

// Warn reports a degraded but continuing state, on stderr.
func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", yellow.Sprint("!"), fmt.Sprintf(format, a...))
}

// Error reports an abort, on stderr. The glyph is added here, never baked into
// the message string.
func Error(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", red.Sprint("✗"), fmt.Sprintf(format, a...))
}

// Hint explains the line above it, dimmed and indented two spaces, on stdout.
func Hint(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "  %s\n", faint.Sprint(fmt.Sprintf(format, a...)))
}

// Dim renders secondary text, for callers that compose their own line.
func Dim(s string) string {
	return faint.Sprint(s)
}

// Table writes aligned columns to stdout. The header is dimmed rather than
// bold: it is scaffolding, and the data is what the eye should land on.
func Table(header []string, rows [][]string) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, faint.Sprint(strings.Join(header, "\t")))
	for _, row := range rows {
		fmt.Fprintln(writer, strings.Join(row, "\t"))
	}
	writer.Flush()
}

// JSON writes one document to stdout and nothing else, per CLI-STANDARD §8.
func JSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// Plain writes a bare line of data to stdout, for piped output.
func Plain(format string, a ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", a...)
}

// Stdout is the data sink, kept behind one name so a future --output flag has
// exactly one place to redirect.
func Stdout() *os.File { return os.Stdout }
