package sqlcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLegacyDataIsMinimizedDuringMigration(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	dataDir := filepath.Join(homeDir, ".infoe-bots")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	const rawUserID = "123456789012345678"
	legacy := legacyData{
		Users: map[string]map[string]time.Time{
			rawUserID: {"llm_promoter": time.Now()},
		},
		LastSent: map[string]map[string]time.Time{
			rawUserID: {"llm_promoter": time.Now()},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dataDir, "opt-outs.json")
	if err := os.WriteFile(dataPath, raw, 0600); err != nil {
		t.Fatal(err)
	}

	if err := Initialize("test-token"); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	optedOut, err := HasOptedOut(rawUserID, "llm_promoter")
	if err != nil {
		t.Fatalf("HasOptedOut() error = %v", err)
	}
	if !optedOut {
		t.Fatal("legacy opt-out was not preserved")
	}

	migrated, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{rawUserID, "LastSent", "Users"} {
		if strings.Contains(string(migrated), forbidden) {
			t.Fatalf("migrated data contains forbidden legacy value %q", forbidden)
		}
	}

	if mode := fileMode(t, dataDir); mode != 0700 {
		t.Fatalf("data directory mode = %04o, want 0700", mode)
	}
	if mode := fileMode(t, dataPath); mode != 0600 {
		t.Fatalf("data file mode = %04o, want 0600", mode)
	}
}

func TestCooldownIsNotPersisted(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	t.Setenv("HOME", t.TempDir())

	if err := Initialize("test-token"); err != nil {
		t.Fatal(err)
	}
	if err := RecordMessageSent("123456789012345678", "llm_promoter"); err != nil {
		t.Fatal(err)
	}
	if CanSendMessage("123456789012345678", "llm_promoter", time.Hour) {
		t.Fatal("cooldown was not retained in memory")
	}
	if _, err := os.Stat(dataFile); !os.IsNotExist(err) {
		t.Fatalf("cooldown created persistent data: %v", err)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestCooldownExpiryRemovesHistoryButPreservesOptOuts(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	t.Setenv("HOME", t.TempDir())
	if err := Initialize("test-token"); err != nil {
		t.Fatal(err)
	}
	if err := AddOptOut("opted-out", "llm_promoter"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mu.Lock()
	lastSent = map[string]map[string]time.Time{
		"expired": {"llm_promoter": now.Add(-CooldownRetention)},
		"recent":  {"llm_promoter": now.Add(-time.Hour), "old-feature": now.Add(-24 * time.Hour)},
	}
	pruneCooldownsLocked(now)
	_, expiredRetained := lastSent["expired"]
	recentCount := len(lastSent["recent"])
	mu.Unlock()
	if expiredRetained {
		t.Fatal("expired pseudonym retained")
	}
	if recentCount != 1 {
		t.Fatal("expected only recent cooldown to remain")
	}
	optedOut, err := HasOptedOut("opted-out", "llm_promoter")
	if err != nil || !optedOut {
		t.Fatalf("opt-out was not preserved: %v", err)
	}
	mu.Lock()
	pruneCooldownsLocked(now.Add(CooldownRetention))
	remaining := len(lastSent)
	mu.Unlock()
	if remaining != 0 {
		t.Fatal("expired history remains")
	}
}
