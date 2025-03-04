package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dixieflatline76/Spice/util/log"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
)

var assetMgr = asset.NewManager()

// EULAPreferenceKey is the key for the EULA acceptance preference.
const EULAPreferenceKey = "eula_acceptance"

// EULAAcceptance is the struct for storing the acceptance of the EULA
type EULAAcceptance struct {
	EULAVersion         string    `json:"eula_version"`
	AcceptanceTimestamp time.Time `json:"acceptance_timestamp"`
	Hash                string    `json:"hash"`
}

// generateEULAHash generates a hash of the EULA text, machine ID, date, and version
func generateEULAHash(eulaText, version string) string {
	machineID := getMachineID()
	dateStr := time.Now().Format("2006-01-02")
	data := fmt.Sprintf("%s%s%s%s", eulaText, machineID, dateStr, version)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// getMachineID gets the machine ID
func getMachineID() string {
	hostname, err := os.Hostname()
	if err != nil {
		// Handle the error appropriately (e.g., log it, return a fallback value)
		return "unknown-host"
	}
	return hostname
}

// HasAcceptedEULA checks if the EULA has been accepted
func HasAcceptedEULA(prefs fyne.Preferences) bool {
	eulaData := prefs.String(EULAPreferenceKey)

	if eulaData == "" {
		return false
	}

	var acceptance EULAAcceptance
	err := json.Unmarshal([]byte(eulaData), &acceptance)
	if err != nil {
		// Handle JSON parsing error
		log.Println("Error parsing EULA acceptance preference:", err)
		return false
	}

	eulaText, _ := assetMgr.GetText("eula.txt")
	currentHash := generateEULAHash(eulaText, config.AppVersion)

	if acceptance.Hash == currentHash && acceptance.EULAVersion == config.AppVersion {
		// EULA was accepted and is valid
		return true
	}
	// EULA not accepted or has been tampered with
	return false
}

// MarkEULAAccepted marks the EULA as accepted
func MarkEULAAccepted(prefs fyne.Preferences) {
	eulaText, _ := assetMgr.GetText("eula.txt")
	hash := generateEULAHash(eulaText, config.AppVersion)

	// Create the acceptance struct
	acceptance := EULAAcceptance{
		EULAVersion:         config.AppVersion,
		AcceptanceTimestamp: time.Now(),
		Hash:                hash,
	}

	jsonData, _ := json.Marshal(acceptance)

	prefs.SetString(EULAPreferenceKey, string(jsonData)) // Save the acceptance data
}
