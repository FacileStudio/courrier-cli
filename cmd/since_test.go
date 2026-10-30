package cmd

import (
	"testing"
	"time"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

type sinceCase struct {
	raw  string
	want time.Time
	bad  bool
}

var sinceNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

var sinceCases = []sinceCase{
	{raw: "30m", want: sinceNow.Add(-30 * time.Minute)},
	{raw: "2h", want: sinceNow.Add(-2 * time.Hour)},
	{raw: "1d", want: sinceNow.Add(-24 * time.Hour)},
	{raw: "2w", want: sinceNow.Add(-14 * 24 * time.Hour)},
	{raw: "2026-10-29T08:00:00Z", want: time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)},
	{raw: "2026-10-29T08:00:00.5+02:00", want: time.Date(2026, 10, 29, 6, 0, 0, 500000000, time.UTC)},
	{raw: "", bad: true},
	{raw: "h", bad: true},
	{raw: "2", bad: true},
	{raw: "-2h", bad: true},
	{raw: "1.5h", bad: true},
	{raw: "2y", bad: true},
	{raw: "yesterday", bad: true},
	{raw: "2026-10-29", bad: true},
}

func TestParseSinceGrammar(t *testing.T) {
	for _, tt := range sinceCases {
		got, err := parseSince(tt.raw, sinceNow)
		if tt.bad != (err != nil) || !tt.bad && !got.Equal(tt.want) {
			t.Errorf("parseSince(%q) = (%v, %v), want %v (bad=%v)", tt.raw, got, err, tt.want, tt.bad)
		}
	}
}

func TestApplySinceFiltersOnDate(t *testing.T) {
	cutoff := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	page := client.EmailPage{Total: 90, Page: 1, Limit: 50, Emails: []client.Email{
		{ID: 1, Date: "2026-10-30T11:00:00Z"},
		{ID: 2, Date: "garbled"},
		{ID: 3, Date: "2026-10-30T09:59:59Z"},
	}}

	applySince(&page, cutoff)

	if len(page.Emails) != 2 || page.Emails[0].ID != 1 || page.Emails[1].ID != 2 {
		t.Fatalf("kept %+v", page.Emails)
	}
	if page.Total != 2 {
		t.Errorf("Total = %d, want 2: nothing newer can follow a dropped row", page.Total)
	}
}

func TestApplySinceKeepsTotalWhenNothingDropped(t *testing.T) {
	page := client.EmailPage{Total: 90, Page: 1, Limit: 50, Emails: []client.Email{{ID: 1, Date: "2026-10-30T11:00:00Z"}}}
	applySince(&page, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if page.Total != 90 {
		t.Errorf("Total = %d, want 90", page.Total)
	}
}
