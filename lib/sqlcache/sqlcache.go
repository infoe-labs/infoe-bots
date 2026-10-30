package sqlcache

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const dataVersion = 2

// persistedData intentionally contains no raw Discord identifiers, message
// content, usernames, timestamps, guild IDs, or channel IDs.
type persistedData struct {
	Version int                 `json:"version"`
	OptOuts map[string][]string `json:"opt_outs"`
}

// legacyData is read only to migrate the original raw-ID format in place.
type legacyData struct {
	Users    map[string]map[string]time.Time `json:"Users"`
	LastSent map[string]map[string]time.Time `json:"LastSent"`
}

var (
	mu            sync.RWMutex
	data          persistedData
	lastSent      map[string]map[string]time.Time
	identifierKey []byte
	dataFile      string
	initialized   bool
)

// Initialize configures privacy-preserving local persistence. The Discord token
// is used only as an HMAC key and is never written by this package. Rotating the
// token intentionally makes the old pseudonyms unlinkable.
func Initialize(discordToken string) error {
	mu.Lock()
	defer mu.Unlock()

	if initialized {
		return nil
	}
	if discordToken == "" {
		return errors.New("identifier key is empty")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.New("resolve local data directory")
	}
	dataDir := filepath.Join(homeDir, ".infoe-bots")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return errors.New("create local data directory")
	}
	if err := os.Chmod(dataDir, 0700); err != nil {
		return errors.New("secure local data directory")
	}

	identifierKey = []byte(discordToken)
	dataFile = filepath.Join(dataDir, "opt-outs.json")
	data = persistedData{Version: dataVersion, OptOuts: make(map[string][]string)}
	lastSent = make(map[string]map[string]time.Time)

	migrated, err := loadLocked()
	if err != nil {
		return err
	}
	if err := os.Chmod(dataFile, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("secure local opt-out data")
	}
	if migrated {
		if err := saveLocked(); err != nil {
			return err
		}
	}
	initialized = true
	return nil
}

func pseudonym(userID string) string {
	digest := hmac.New(sha256.New, identifierKey)
	_, _ = digest.Write([]byte("discord-user:" + userID))
	return hex.EncodeToString(digest.Sum(nil))
}

func loadLocked() (bool, error) {
	raw, err := os.ReadFile(dataFile)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("read local opt-out data")
	}

	var versionProbe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &versionProbe); err != nil {
		return false, errors.New("decode local opt-out data")
	}
	if versionProbe.Version == dataVersion {
		if err := json.Unmarshal(raw, &data); err != nil {
			return false, errors.New("decode local opt-out data")
		}
		if data.OptOuts == nil {
			data.OptOuts = make(map[string][]string)
		}
		return false, nil
	}

	var legacy legacyData
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return false, errors.New("decode legacy opt-out data")
	}
	for userID, features := range legacy.Users {
		key := pseudonym(userID)
		for feature := range features {
			data.OptOuts[key] = appendUnique(data.OptOuts[key], feature)
		}
	}
	// Legacy LastSent timestamps are deliberately discarded instead of migrated.
	return true, nil
}

func saveLocked() error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.New("encode local opt-out data")
	}

	tempFile, err := os.CreateTemp(filepath.Dir(dataFile), ".opt-outs-*")
	if err != nil {
		return errors.New("create temporary opt-out data")
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return errors.New("secure temporary opt-out data")
	}
	if _, err := tempFile.Write(raw); err != nil {
		tempFile.Close()
		return errors.New("write temporary opt-out data")
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return errors.New("sync temporary opt-out data")
	}
	if err := tempFile.Close(); err != nil {
		return errors.New("close temporary opt-out data")
	}
	if err := os.Rename(tempName, dataFile); err != nil {
		return errors.New("replace local opt-out data")
	}
	return nil
}

func requireInitialized() error {
	if !initialized {
		return errors.New("privacy store is not initialized")
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// AddOptOut persists only a token-keyed pseudonym and feature name.
func AddOptOut(userID, feature string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := requireInitialized(); err != nil {
		return err
	}

	key := pseudonym(userID)
	data.OptOuts[key] = appendUnique(data.OptOuts[key], feature)
	return saveLocked()
}

func HasOptedOut(userID, feature string) (bool, error) {
	mu.RLock()
	defer mu.RUnlock()
	if err := requireInitialized(); err != nil {
		return false, err
	}

	for _, optedOutFeature := range data.OptOuts[pseudonym(userID)] {
		if optedOutFeature == feature {
			return true, nil
		}
	}
	return false, nil
}

func RemoveOptOut(userID, feature string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := requireInitialized(); err != nil {
		return err
	}

	key := pseudonym(userID)
	features := data.OptOuts[key]
	filtered := features[:0]
	for _, existing := range features {
		if existing != feature {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) == 0 {
		delete(data.OptOuts, key)
	} else {
		data.OptOuts[key] = filtered
	}
	return saveLocked()
}

// RecordMessageSent retains cooldown state in memory only. It is discarded on
// every restart and is never included in the persistent opt-out file.
func RecordMessageSent(userID, feature string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := requireInitialized(); err != nil {
		return err
	}

	key := pseudonym(userID)
	if lastSent[key] == nil {
		lastSent[key] = make(map[string]time.Time)
	}
	lastSent[key][feature] = time.Now()
	return nil
}

func CanSendMessage(userID, feature string, cooldown time.Duration) bool {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		return false
	}

	last, exists := lastSent[pseudonym(userID)][feature]
	return !exists || time.Since(last) > cooldown
}

// resetForTest prevents test state from crossing test cases.
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	data = persistedData{}
	lastSent = nil
	identifierKey = nil
	dataFile = ""
	initialized = false
}
