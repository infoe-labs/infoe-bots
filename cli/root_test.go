package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenFileLoading(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	tokenFile := filepath.Join(homeDir, "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0600); err != nil {
		t.Fatal(err)
	}

	DiscordToken = ""
	discordTokenFile = tokenFile
	t.Cleanup(func() {
		DiscordToken = ""
		discordTokenFile = ""
	})

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE() error = %v", err)
	}
	if DiscordToken != "test-token" {
		t.Fatalf("DiscordToken = %q, want %q", DiscordToken, "test-token")
	}
}

func TestTokenFileMustBePrivate(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0644); err != nil {
		t.Fatal(err)
	}

	DiscordToken = ""
	discordTokenFile = tokenFile
	t.Cleanup(func() {
		DiscordToken = ""
		discordTokenFile = ""
	})

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err == nil {
		t.Fatal("PersistentPreRunE() accepted an overly permissive token file")
	}
}
