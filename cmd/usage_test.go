package cmd

import (
	"testing"
)

func resetUsageFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		flagAccount, flagTriageAll, flagTriageClass, flagTriageSince = 0, false, "", ""
		flagTriageLimit, flagTriagePage, flagContactsLimit, flagContactsPage = 50, 1, 50, 1
		flagListLimit, flagListPage, flagListSince, flagContactsAll = 50, 1, "", false
		commandStarted = false
	})
}

var badFlagCases = []struct {
	name string
	set  func()
	run  func(*testing.T) error
}{
	{"triage since", func() { flagTriageSince = "yesterday" }, runTriagePre},
	{"triage class", func() { flagTriageClass = "spam" }, runTriagePre},
	{"triage all and account", func() { flagTriageAll, flagAccount = true, 3 }, runTriagePre},
	{"triage limit 0", func() { flagTriageLimit = 0 }, runTriagePre},
	{"triage limit negative", func() { flagTriageLimit = -5 }, runTriagePre},
	{"triage page 0", func() { flagTriagePage = 0 }, runTriagePre},
	{"contacts limit 0", func() { flagContactsLimit = 0 }, runContactsPre},
	{"contacts page 0", func() { flagContactsPage = 0 }, runContactsPre},
	{"inbox limit 0", func() { flagListLimit = 0 }, runInboxPre},
	{"list page 0", func() { flagListPage = 0 }, runListPre},
	{"list since", func() { flagListSince = "2y" }, runListPre},
}

func TestBadFlagsAreUsageErrorsEvenLoggedOut(t *testing.T) {
	t.Setenv("COURRIER_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	resetUsageFlags(t)

	for _, tt := range badFlagCases {
		t.Run(tt.name, func(t *testing.T) {
			resetUsageFlags(t)
			tt.set()
			commandStarted = true
			if err := tt.run(t); err == nil {
				t.Fatal("want a usage error, got nil")
			}
			if commandStarted {
				t.Error("commandStarted is true, so Execute would exit 1 instead of 2")
			}
		})
	}
}

func runTriagePre(*testing.T) error   { return triageCmd.PreRunE(triageCmd, nil) }
func runContactsPre(*testing.T) error { return contactsCmd.PreRunE(contactsCmd, nil) }
func runInboxPre(*testing.T) error    { return inboxCmd.PreRunE(inboxCmd, nil) }
func runListPre(*testing.T) error     { return listCmd.PreRunE(listCmd, []string{"inbox"}) }

func TestGoodFlagsPassTheUsageCheck(t *testing.T) {
	resetUsageFlags(t)
	if err := triageCmd.PreRunE(triageCmd, nil); err != nil {
		t.Errorf("defaults rejected: %v", err)
	}
}
