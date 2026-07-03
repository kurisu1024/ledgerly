package cli

import (
	"bytes"
	"testing"
)

func TestConnectionServerURLPrecedence(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"flag wins over env", "https://flag.example", "https://env.example", "https://flag.example"},
		{"env used when flag empty", "", "https://env.example", "https://env.example"},
		{"empty when neither set", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvServerURL, tt.env)
			c := &connection{serverURLFlag: tt.flag}
			if got := c.serverURL(); got != tt.want {
				t.Fatalf("serverURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConnectionTokenPrecedence(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"flag wins over env", "flag-token", "env-token", "flag-token"},
		{"env used when flag empty", "", "env-token", "env-token"},
		{"empty when neither set", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvToken, tt.env)
			c := &connection{tokenFlag: tt.flag}
			if got := c.token(); got != tt.want {
				t.Fatalf("token() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRootCmdUnknownCommand(t *testing.T) {
	root := NewRootCmd(DefaultDeps())
	root.SetArgs([]string{"bogus-command"})

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)

	if err := root.Execute(); err == nil {
		t.Fatalf("expected usage error for unknown command, got nil")
	}
}

func TestRootCmdRegistersSubcommands(t *testing.T) {
	root := NewRootCmd(DefaultDeps())
	want := []string{"token", "events", "export", "verify"}

	for _, name := range want {
		found := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected root command to register %q subcommand", name)
		}
	}
}
