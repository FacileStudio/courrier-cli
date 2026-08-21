package config

import (
	"os"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := Config{URL: "https://mail.example.test", Token: "tok-123", DefaultAccount: 7}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("round trip: got %+v, want %+v", got, want)
	}
}

func TestSaveUsesOwnerOnlyModes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(Config{URL: DefaultURL, Token: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	file, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := file.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode: got %#o, want 0600", perm)
	}

	dir, err := os.Stat(Dir())
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dir.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode: got %#o, want 0700", perm)
	}
}

func TestLoadTightensLoosePermissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(Config{URL: "https://mail.example.test", Token: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.Chmod(Path(), 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token != "secret" || got.URL != "https://mail.example.test" {
		t.Errorf("content after tightening: got %+v", got)
	}

	info, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode after Load: got %#o, want 0600", perm)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.URL != DefaultURL {
		t.Errorf("URL: got %q, want %q", got.URL, DefaultURL)
	}
	if got.Token != "" || got.DefaultAccount != 0 {
		t.Errorf("expected empty credentials, got %+v", got)
	}
}

func TestLoadEmptyURLFallsBackToDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(Config{Token: "tok"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.URL != DefaultURL {
		t.Errorf("URL: got %q, want %q", got.URL, DefaultURL)
	}
}

func TestClearKeepsURLAndAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Save(Config{URL: "https://mail.example.test", Token: "tok", DefaultAccount: 3}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{URL: "https://mail.example.test", DefaultAccount: 3}
	if got != want {
		t.Fatalf("after Clear: got %+v, want %+v", got, want)
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"courrier.facile.studio", "https://courrier.facile.studio"},
		{"https://courrier.facile.studio/", "https://courrier.facile.studio"},
		{"courrier.facile.studio/", "https://courrier.facile.studio"},
		{"http://localhost:4000", "http://localhost:4000"},
		{"", ""},
		{"   ", ""},
	}

	for _, c := range cases {
		if got := NormalizeURL(c.in); got != c.want {
			t.Errorf("NormalizeURL(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

// TestClearSurvivesAnUnparseableFile pins the property that matters most about
// logout: a credential that cannot be parsed must still be removed. Refusing
// because the YAML is malformed leaves a working token exactly where somebody
// tried to delete it.
func TestClearSurvivesAnUnparseableFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("url: https://courrier.facile.studio\ntoken: still-valid\n  bad: [unclosed\n")
	if err := os.WriteFile(Path(), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Clear(); err != nil {
		t.Fatalf("Clear on an unparseable file: %v", err)
	}

	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "still-valid") {
		t.Fatalf("the token survived logout: %s", data)
	}
}

// TestSaveTightensAnExistingLooseFile guards the gap between OpenFile's perm
// argument, which applies only at creation, and an existing file that already
// carries a looser mode.
func TestSaveTightensAnExistingLooseFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte("url: https://old.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(Config{URL: "https://new.example.com", Token: "t"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode is %o after Save, want 600", got)
	}
}
