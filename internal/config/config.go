// Package config stores which Courrier instance the CLI talks to, the session it
// holds for it, and the mailbox it defaults to.
package config

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultURL is where the suite's shared Courrier instance listens.
const DefaultURL = "https://courrier.facile.studio"

// Config is the whole stored state. It is deliberately small: everything else
// lives in the instance. Callers read-modify-write it, so keys written by a
// future version survive a round trip through an older binary.
type Config struct {
	URL            string `yaml:"url"`
	Token          string `yaml:"token,omitempty"`
	DefaultAccount int64  `yaml:"default_account,omitempty"`
}

// Dir is the configuration directory, honouring XDG_CONFIG_HOME and falling back
// to a relative dotdir when the home directory cannot be resolved.
func Dir() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "courrier")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".courrier"
	}
	return filepath.Join(home, ".config", "courrier")
}

// Path is the configuration file.
func Path() string { return filepath.Join(Dir(), "config.yml") }

// Load reads the configuration, returning defaults when none exists yet so a
// first run is not an error. A stored file with no URL falls back to DefaultURL.
//
// It also tightens the file to 0600 when it is found with any group or other bit
// set. Installs predate the permission rule, and a tool that only writes
// correctly leaves those tokens exposed forever.
func Load() (Config, error) {
	cfg := Config{URL: DefaultURL}

	f, err := os.Open(Path())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return cfg, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := f.Chmod(0o600); err != nil {
			return cfg, err
		}
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	return cfg, nil
}

// Save writes the configuration owner-only. The file holds a bearer token, so
// the mode is set at creation rather than chmod'd afterwards: writing first and
// fixing the mode second leaves a window in which the token is world-readable,
// and on a shared machine that window is the whole attack. The directory is
// created 0700 for the same reason.
func Save(cfg Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(Path(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Clear removes the stored session but keeps the instance URL and the default
// account, so logging out does not also make the user retype both.
//
// A file that cannot be parsed still loses its token. Logout is what somebody
// reaches for on a borrowed machine, and refusing because the YAML is malformed
// would leave a working credential exactly where they tried to remove it. The
// other fields are unrecoverable in that case, so they reset to defaults.
func Clear() error {
	cfg, err := Load()
	if err != nil {
		cfg = Config{URL: DefaultURL}
	}
	cfg.Token = ""
	return Save(cfg)
}

// NormalizeURL trims a trailing slash and supplies a scheme, so
// `courrier login courrier.facile.studio` works as typed. An empty input stays
// empty rather than becoming a bare scheme.
func NormalizeURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return "https://" + trimmed
	}
	return trimmed
}
