package cmd

import (
	"reflect"
	"testing"
)

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "", want: nil},
		{input: "   ", want: nil},
		{input: "https://example.com", want: []string{"https://example.com"}},
		{input: "https://a.com, https://b.com", want: []string{"https://a.com", "https://b.com"}},
		{input: " https://a.com , , https://b.com ", want: []string{"https://a.com", "https://b.com"}},
	}

	for _, tt := range tests {
		got := parseOrigins(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseOrigins(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKeysCommandTree(t *testing.T) {
	subcommands := keysCmd.Commands()
	names := make(map[string]bool)
	for _, cmd := range subcommands {
		names[cmd.Name()] = true
	}

	for _, want := range []string{"list", "create", "revoke"} {
		if !names[want] {
			t.Errorf("keysCmd missing subcommand %q", want)
		}
	}
}
