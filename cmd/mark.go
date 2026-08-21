package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FacileStudio/courrier-cli/internal/client"
	"github.com/FacileStudio/courrier-cli/internal/ui"
)

// maxMarkIDs is the instance's ceiling for one batch. internal/client refuses a
// larger one too, but a message about a bulk action's arguments reads like a
// server complaint; caught here it reads like the advice it is.
const maxMarkIDs = 200

var (
	flagMarkRead    bool
	flagMarkUnread  bool
	flagMarkStar    bool
	flagMarkUnstar  bool
	flagMarkArchive bool
	flagMarkDelete  bool
	flagMarkYes     bool
)

var markCmd = &cobra.Command{
	Use:   "mark <id>...",
	Short: "Change the state of messages",
	Long: `Change the state of one or more messages.

Ids are numeric email ids from a listing: the ID column of courrier inbox, or
the id field of courrier inbox --json. Exactly one action applies per run, and
at most 200 ids.

Deleting cannot be undone, so it asks first on a terminal. Pass --yes to skip
the question; --yes is required when stdin is not a terminal, because a prompt
nothing can answer is a hang, not a safeguard.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMark,
}

// init wires the mark flags. They are checked in the command body rather than
// through cobra's flag groups: those report after the run hooks have started,
// so they exit 1 with a message in cobra's voice instead of 2 in this CLI's.
func init() {
	markCmd.Flags().BoolVar(&flagMarkRead, "read", false, "Mark the messages read")
	markCmd.Flags().BoolVar(&flagMarkUnread, "unread", false, "Mark the messages unread")
	markCmd.Flags().BoolVar(&flagMarkStar, "star", false, "Star the messages")
	markCmd.Flags().BoolVar(&flagMarkUnstar, "unstar", false, "Remove the star from the messages")
	markCmd.Flags().BoolVar(&flagMarkArchive, "archive", false, "Move the messages to the archive folder")
	markCmd.Flags().BoolVar(&flagMarkDelete, "delete", false, "Delete the messages")
	markCmd.Flags().BoolVar(&flagMarkYes, "yes", false, "Answer the confirmation with yes")
}

// starOn and starOff back the pointers the star flags hand to UpdateEmail, which
// takes a *bool so that a nil leaves the flag alone.
var starOn, starOff = true, false

// markChoice is one action flag and what it does. bulk names the server-side
// bulk action and is empty for the two star flags, which have no bulk
// equivalent; starred is set for exactly those two. verb completes the sentence
// "3 messages <verb>".
type markChoice struct {
	flag    string
	set     *bool
	bulk    string
	starred *bool
	verb    string
}

// markChoices is the action surface, in the order the help lists it.
var markChoices = []markChoice{
	{flag: "--read", set: &flagMarkRead, bulk: "mark_read", verb: "marked read"},
	{flag: "--unread", set: &flagMarkUnread, bulk: "mark_unread", verb: "marked unread"},
	{flag: "--star", set: &flagMarkStar, starred: &starOn, verb: "starred"},
	{flag: "--unstar", set: &flagMarkUnstar, starred: &starOff, verb: "unstarred"},
	{flag: "--archive", set: &flagMarkArchive, bulk: "archive", verb: "archived"},
	{flag: "--delete", set: &flagMarkDelete, bulk: "delete", verb: "deleted"},
}

// markReport is the --json document for a mark run.
type markReport struct {
	AccountID int64   `json:"account_id"`
	Action    string  `json:"action"`
	Changed   []int64 `json:"changed"`
	Failed    []int64 `json:"failed"`
}

// runMark validates the request before touching the network, because a mistyped
// id or a missing action does not need a round trip to be wrong.
func runMark(cmd *cobra.Command, args []string) error {
	ctx, stop := signalContext()
	defer stop()

	choice, err := chosenMark()
	if err != nil {
		return err
	}
	ids, err := parseEmailIDs(args)
	if err != nil {
		return err
	}

	api, cfg, err := session()
	if err != nil {
		return err
	}
	accountID, err := resolveAccount(ctx, api, cfg)
	if err != nil {
		return err
	}

	if choice.bulk == "delete" {
		if err := confirmDelete(ids); err != nil {
			return err
		}
	}

	if choice.starred != nil {
		return markStarred(ctx, api, accountID, ids, choice)
	}
	return markBulk(ctx, api, accountID, ids, choice)
}

// chosenMark picks the single action the flags ask for. None and several are
// both usage errors, and both name the whole set: a reader who passed the wrong
// flag needs to see the right one, not be told the count was wrong.
func chosenMark() (markChoice, error) {
	chosen := make([]markChoice, 0, 1)
	for _, choice := range markChoices {
		if *choice.set {
			chosen = append(chosen, choice)
		}
	}

	switch len(chosen) {
	case 1:
		return chosen[0], nil
	case 0:
		return markChoice{}, fmt.Errorf("mark needs an action — pass one of %s", markFlagList())
	}

	names := make([]string, 0, len(chosen))
	for _, choice := range chosen {
		names = append(names, choice.flag)
	}
	return markChoice{}, fmt.Errorf("%s cannot be combined — pass exactly one of %s", strings.Join(names, " and "), markFlagList())
}

// markFlagList renders the action flags for an error message.
func markFlagList() string {
	names := make([]string, 0, len(markChoices))
	for _, choice := range markChoices {
		names = append(names, choice.flag)
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// parseEmailIDs turns the positional arguments into email ids, refusing the
// batch whole rather than acting on the prefix that happened to parse.
func parseEmailIDs(args []string) ([]int64, error) {
	if len(args) > maxMarkIDs {
		return nil, fmt.Errorf("%d ids is more than the %d the instance accepts at once — split the list into smaller batches", len(args), maxMarkIDs)
	}

	ids := make([]int64, 0, len(args))
	for _, arg := range args {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("%q is not an email id — ids are the numbers in the ID column of `courrier inbox`", arg)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// confirmDelete asks before a delete that nothing undoes.
//
// The gate is stdin, not stdout: a question is only a safeguard when something
// can answer it, and a pipeline, a CI job and an agent all cannot. They get the
// flag that unblocks them instead of a prompt that hangs until their timeout.
// Structured output never prompts at all, so --json without --yes is refused
// rather than silently confirmed.
func confirmDelete(ids []int64) error {
	if flagMarkYes {
		return nil
	}
	if flagJSON {
		return errors.New("--delete with --json needs --yes — structured output never prompts")
	}
	if !stdinIsTerminal() {
		return errors.New("--delete needs --yes when stdin is not a terminal — nothing there can answer the confirmation")
	}

	ui.Warn("about to permanently delete %s: %s", messageCount(len(ids)), joinIDs(ids))
	confirmed, err := confirmAction("delete them?")
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("delete cancelled — nothing was changed")
	}
	return nil
}

// markBulk applies one server-side action to the whole batch in a single call,
// which is what the bulk endpoint exists for and what keeps a batch of 200 off
// the rate limiter.
func markBulk(ctx context.Context, api *client.Client, accountID int64, ids []int64, choice markChoice) error {
	if err := api.BulkAction(ctx, accountID, ids, choice.bulk); err != nil {
		if ctx.Err() != nil {
			return ErrInterrupted
		}
		if rateLimited(err) {
			return fmt.Errorf("the instance refused the change under its rate limit — wait a minute and run it again")
		}
		return fmt.Errorf("%s could not be %s — %w", messageCount(len(ids)), choice.verb, err)
	}
	return reportMark(accountID, ids, nil, choice)
}

// markStarred flips the star one message at a time: the bulk endpoint has no
// star action. Each failure is named as it happens and the run still exits
// non-zero, so a batch that half worked is not mistaken for one that worked.
func markStarred(ctx context.Context, api *client.Client, accountID int64, ids []int64, choice markChoice) error {
	failed := make([]int64, 0)

	for _, id := range ids {
		if _, err := api.UpdateEmail(ctx, accountID, id, nil, choice.starred); err != nil {
			if ctx.Err() != nil {
				return ErrInterrupted
			}
			ui.Warn("email %d could not be %s — %s", id, choice.verb, err)
			failed = append(failed, id)
		}
	}
	return reportMark(accountID, ids, failed, choice)
}

// reportMark prints the outcome — one JSON document under --json, a success line
// otherwise — and turns a partial failure into a non-zero exit.
func reportMark(accountID int64, ids, failed []int64, choice markChoice) error {
	if failed == nil {
		failed = []int64{}
	}
	changed := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(failed, id) {
			changed = append(changed, id)
		}
	}

	if flagJSON {
		err := ui.JSON(markReport{
			AccountID: accountID,
			Action:    strings.TrimPrefix(choice.flag, "--"),
			Changed:   changed,
			Failed:    failed,
		})
		if err != nil {
			return err
		}
	} else if len(changed) > 0 {
		ui.Success("%s %s", messageCount(len(changed)), choice.verb)
	}

	if len(failed) > 0 {
		return fmt.Errorf("%s could not be %s (%s) — retry those ids", messageCount(len(failed)), choice.verb, joinIDs(failed))
	}
	return nil
}

// messageCount renders a batch size, so one message does not read as
// "1 messages".
func messageCount(n int) string {
	if n == 1 {
		return "1 message"
	}
	return fmt.Sprintf("%d messages", n)
}

// joinIDs renders an id list for a message, cut short past twenty because a
// confirmation the reader has to scroll is one they stop reading.
func joinIDs(ids []int64) string {
	const shown = 20

	parts := make([]string, 0, min(len(ids), shown))
	for _, id := range ids[:min(len(ids), shown)] {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	joined := strings.Join(parts, ", ")
	if len(ids) > shown {
		joined += fmt.Sprintf(", and %d more", len(ids)-shown)
	}
	return joined
}

// confirmAction asks a yes/no question and reads the answer, treating anything
// but an explicit yes as no.
//
// It is the one place in the command tree that writes outside internal/ui: a
// prompt ends without a newline so the answer is typed on the same line, and ui
// is line-oriented by design. The question goes to stderr with the rest of the
// non-data output.
func confirmAction(question string) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("cannot read the answer — %w", err)
	}

	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
