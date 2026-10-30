package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenFileLoading(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
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

func TestTokenSourcesAreMutuallyExclusive(t *testing.T) {
	DiscordToken = "inline-token"
	discordTokenFile = "/unused"
	t.Cleanup(func() {
		DiscordToken = ""
		discordTokenFile = ""
	})

	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err == nil {
		t.Fatal("PersistentPreRunE() accepted two token sources")
	}
}
