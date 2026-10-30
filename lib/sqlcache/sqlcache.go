package sqlcache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OptOutData represents the structure of our opt-out data
type OptOutData struct {
	Users    map[string]map[string]time.Time // map[userID]map[feature]timestamp
	LastSent map[string]map[string]time.Time // map[userID]map[feature]lastMessageTimestamp
}

var (
	optOutData OptOutData
	fileMutex  sync.RWMutex
	dataFile   string
	once       sync.Once
)

// Initialize creates and initializes the file-based persistence
func Initialize() {
	once.Do(func() {
		// Create data directory if it doesn't exist
		dataDir := filepath.Join(os.Getenv("HOME"), ".infoe-bots")
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return
		}

		// Set the data file path
		dataFile = filepath.Join(dataDir, "opt-outs.json")

		// Initialize the opt-out data structure
		optOutData = OptOutData{
			Users:    make(map[string]map[string]time.Time),
			LastSent: make(map[string]map[string]time.Time),
		}

		// Load existing data if available
		loadData()
	})
}

// loadData loads opt-out data from the JSON file
func loadData() {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	// Check if the file exists
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		// File doesn't exist, nothing to load
		return
	}

	// Read the file
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return
	}

	// Parse the JSON data
	if err := json.Unmarshal(data, &optOutData); err != nil {
		return
	}
}

// saveDataLocked saves opt-out data while the caller holds fileMutex.
func saveDataLocked() error {
	// Convert data to JSON
	data, err := json.MarshalIndent(optOutData, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(dataFile, data, 0600); err != nil {
		return err
	}

	return nil
}

// AddOptOut adds a user to the opt-out list for a specific feature
func AddOptOut(userID, feature string) error {
	Initialize()

	fileMutex.Lock()
	defer fileMutex.Unlock()

	// Initialize user's feature map if it doesn't exist
	if optOutData.Users[userID] == nil {
		optOutData.Users[userID] = make(map[string]time.Time)
	}

	// Add the opt-out with current timestamp
	optOutData.Users[userID][feature] = time.Now()

	// Save to file
	err := saveDataLocked()
	if err != nil {
		return err
	}
	return nil
}

// HasOptedOut checks if a user has opted out of a specific feature
func HasOptedOut(userID, feature string) (bool, error) {
	Initialize()

	fileMutex.RLock()
	defer fileMutex.RUnlock()

	// Check if user exists in opt-out data
	userFeatures, exists := optOutData.Users[userID]
	if !exists {
		return false, nil
	}

	// Check if feature exists in user's opt-outs
	_, optedOut := userFeatures[feature]
	return optedOut, nil
}

// RemoveOptOut removes a user from the opt-out list for a specific feature
func RemoveOptOut(userID, feature string) error {
	Initialize()

	fileMutex.Lock()
	defer fileMutex.Unlock()

	// Check if user exists in opt-out data
	if userFeatures, exists := optOutData.Users[userID]; exists {
		// Remove the feature from user's opt-outs
		delete(userFeatures, feature)

		// If user has no more opt-outs, remove the user
		if len(userFeatures) == 0 {
			delete(optOutData.Users, userID)
		}

		// Save to file
		err := saveDataLocked()
		if err != nil {
			return err
		}
	}
	return nil
}

// ListOptOuts lists all opt-outs for debugging
func ListOptOuts() map[string][]string {
	fileMutex.RLock()
	defer fileMutex.RUnlock()

	result := make(map[string][]string)
	for userID, features := range optOutData.Users {
		featureList := []string{}
		for feature := range features {
			featureList = append(featureList, feature)
		}
		result[userID] = featureList
	}
	return result
}

// RecordMessageSent records when a message was sent to a user for a specific feature
func RecordMessageSent(userID, feature string) error {
	Initialize()

	fileMutex.Lock()
	defer fileMutex.Unlock()

	// Initialize user's feature map if it doesn't exist
	if optOutData.LastSent[userID] == nil {
		optOutData.LastSent[userID] = make(map[string]time.Time)
	}

	// Record current time
	optOutData.LastSent[userID][feature] = time.Now()

	// Save to file
	return saveDataLocked()
}

// CanSendMessage checks if enough time has passed since the last message
// Returns true if a message can be sent (cooldown period has passed or no previous message)
func CanSendMessage(userID, feature string, cooldownDuration time.Duration) bool {
	Initialize()

	fileMutex.RLock()
	defer fileMutex.RUnlock()

	// Check if user exists in last sent data
	userFeatures, exists := optOutData.LastSent[userID]
	if !exists {
		// No record of previous messages, can send
		return true
	}

	// Check if feature exists in user's last sent
	lastSent, exists := userFeatures[feature]
	if !exists {
		// No record for this feature, can send
		return true
	}

	// Check if cooldown period has passed
	return time.Since(lastSent) > cooldownDuration
}
