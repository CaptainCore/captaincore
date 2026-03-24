package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SystemConfig holds the system-level configuration (first entry in config.json).
type SystemConfig struct {
	CaptainCoreFleet      string `json:"captaincore_fleet"`
	CaptainCoreDev        string `json:"captaincore_dev"`
	CaptainCoreMaster     string `json:"captaincore_master"`
	CaptainCoreMasterPort string `json:"captaincore_master_port"`
	Logs                  string `json:"logs"`
	Path                  string `json:"path"`
	PathTmp               string `json:"path_tmp"`
	PathScripts           string `json:"path_scripts"`
	PathKeys              string `json:"path_keys"`
	PathRecipes           string `json:"path_recipes"`
	RcloneBackup          string `json:"rclone_backup"`
	RcloneCLIBackup       string `json:"rclone_cli_backup"`
	RcloneSnapshot        string `json:"rclone_snapshot"`
	RcloneUpload          string `json:"rclone_upload"`
	RcloneUploadURI       string `json:"rclone_upload_uri"`
	FathomAPIKey          string `json:"fathom_api_key"`
	LocalWPDBPW           string `json:"local_wp_db_pw"`
	CaptainCoreStandby    string `json:"captaincore_standby"`
}

// CaptainConfig holds a captain-specific configuration entry from config.json.
type CaptainConfig struct {
	CaptainID string                       `json:"captain_id,omitempty"`
	System    *SystemConfig                `json:"system,omitempty"`
	SystemRaw map[string]interface{}       `json:"-"` // all system fields for dynamic KEY=VALUE output
	Keys      map[string]string            `json:"keys,omitempty"`
	Remotes   map[string]string            `json:"remotes,omitempty"`
	Vars      map[string]json.RawMessage   `json:"vars,omitempty"`
}

// MarshalJSON implements custom JSON marshaling for CaptainConfig.
// When SystemRaw is populated, it merges typed SystemConfig fields with any
// extra fields from the original JSON, preventing unknown system fields
// (like rclone_archive) from being silently dropped during save.
func (c CaptainConfig) MarshalJSON() ([]byte, error) {
	if c.System != nil && c.SystemRaw != nil {
		// Serialize typed SystemConfig to get current known fields
		typedBytes, err := json.Marshal(c.System)
		if err != nil {
			return nil, err
		}
		var typedMap map[string]interface{}
		if err := json.Unmarshal(typedBytes, &typedMap); err != nil {
			return nil, err
		}

		// Start with SystemRaw (preserves unknown fields), overlay typed fields
		merged := make(map[string]interface{})
		for k, v := range c.SystemRaw {
			merged[k] = v
		}
		for k, v := range typedMap {
			merged[k] = v
		}

		// Build the output map manually to control field order
		out := make(map[string]interface{})
		out["system"] = merged
		if c.CaptainID != "" {
			out["captain_id"] = c.CaptainID
		}
		if len(c.Keys) > 0 {
			out["keys"] = c.Keys
		}
		if len(c.Remotes) > 0 {
			out["remotes"] = c.Remotes
		}
		if len(c.Vars) > 0 {
			out["vars"] = c.Vars
		}
		return json.Marshal(out)
	}

	// No SystemRaw — use default behavior via alias to avoid recursion
	type Alias CaptainConfig
	return json.Marshal((Alias)(c))
}

// FullConfig represents the entire config.json array.
type FullConfig []CaptainConfig

// LoadConfig reads and parses config.json from the user's home directory.
func LoadConfig() (FullConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return LoadConfigFrom(filepath.Join(home, ".captaincore", "config.json"))
}

// LoadConfigFrom reads and parses config.json from a specific path.
func LoadConfigFrom(path string) (FullConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var configs FullConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}

	// Second pass: populate SystemRaw with all system fields for dynamic output.
	// The typed SystemConfig only has a subset of fields; bash scripts need all of them.
	var rawEntries []map[string]json.RawMessage
	if json.Unmarshal(data, &rawEntries) == nil {
		for i, raw := range rawEntries {
			if i >= len(configs) {
				break
			}
			if sysRaw, ok := raw["system"]; ok {
				var sysMap map[string]interface{}
				if json.Unmarshal(sysRaw, &sysMap) == nil {
					configs[i].SystemRaw = sysMap
				}
			}
		}
	}

	return configs, nil
}

// GetSystem returns the system configuration (first entry with system key).
func (c FullConfig) GetSystem() *SystemConfig {
	for _, entry := range c {
		if entry.System != nil {
			return entry.System
		}
	}
	return nil
}

// GetSystemRaw returns the raw system map (all fields) for dynamic KEY=VALUE output.
func (c FullConfig) GetSystemRaw() map[string]interface{} {
	for _, entry := range c {
		if entry.SystemRaw != nil {
			return entry.SystemRaw
		}
	}
	return nil
}

// GetCaptain returns the captain config matching the given captain ID.
func (c FullConfig) GetCaptain(captainID string) *CaptainConfig {
	for i, entry := range c {
		if entry.CaptainID == captainID {
			return &c[i]
		}
	}
	return nil
}

// FetchCaptainIDs returns a space-separated string of all captain IDs.
// Matches the output of `configs.php fetch-captain-ids`.
func FetchCaptainIDs(configs FullConfig) string {
	var ids []string
	for _, entry := range configs {
		if entry.CaptainID != "" {
			ids = append(ids, entry.CaptainID)
		}
	}
	return strings.Join(ids, " ")
}

// FetchKeyValues returns KEY=VALUE pairs for the given captain ID.
// This replicates the output of `configs.php fetch` exactly.
func FetchKeyValues(configs FullConfig, captainID string) (string, error) {
	system := configs.GetSystem()
	if system == nil {
		return "", fmt.Errorf("Error: System configuration not found")
	}

	systemRaw := configs.GetSystemRaw()
	if systemRaw == nil {
		return "", fmt.Errorf("Error: System configuration not found")
	}

	captain := configs.GetCaptain(captainID)
	if captain == nil {
		return "", fmt.Errorf("Error: Captain not found.")
	}

	var output strings.Builder

	// Adjust paths for fleet mode (matches PHP configs.php behavior)
	if system.CaptainCoreFleet == "true" {
		systemRaw["path"] = fmt.Sprintf("%s/%s", system.Path, captainID)
		if v, ok := systemRaw["rclone_backup"].(string); ok {
			systemRaw["rclone_backup"] = fmt.Sprintf("%s/%s", v, captainID)
		}
		if v, ok := systemRaw["rclone_logs"].(string); ok {
			systemRaw["rclone_logs"] = fmt.Sprintf("%s/%s", v, captainID)
		}
		if v, ok := systemRaw["rclone_snapshot"].(string); ok {
			systemRaw["rclone_snapshot"] = fmt.Sprintf("%s/%s", v, captainID)
		}
	}

	// System key=value pairs — output ALL scalar fields dynamically (matches PHP behavior)
	writeRawMapKV(&output, systemRaw)

	// Captain sub-object key=value pairs (keys, remotes, vars)
	// Skip non-object fields (captain_id is a string, not an object)
	writeMapKV(&output, captain.Keys)
	writeMapKV(&output, captain.Remotes)
	writeVarsKV(&output, captain.Vars)

	return output.String(), nil
}

// FetchSpecificKey returns a specific key's value, e.g. `configs.php fetch vars captaincore_api`.
func FetchSpecificKey(configs FullConfig, captainID, section, key string) (string, error) {
	captain := configs.GetCaptain(captainID)
	if captain == nil {
		return "", fmt.Errorf("Error: Captain not found.")
	}

	switch section {
	case "keys":
		if v, ok := captain.Keys[key]; ok {
			return v, nil
		}
	case "remotes":
		if v, ok := captain.Remotes[key]; ok {
			return v, nil
		}
	case "vars":
		if v, ok := captain.Vars[key]; ok {
			return rawToString(v), nil
		}
	}
	return "", nil
}

// FetchSection returns JSON for an entire section (keys, remotes, vars).
func FetchSection(configs FullConfig, captainID, section string) (string, error) {
	captain := configs.GetCaptain(captainID)
	if captain == nil {
		return "", fmt.Errorf("Error: Captain not found.")
	}

	var data interface{}
	switch section {
	case "keys":
		data = captain.Keys
	case "remotes":
		data = captain.Remotes
	case "vars":
		data = captain.Vars
	default:
		return "", nil
	}

	out, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// writeRawMapKV outputs KEY=VALUE for all scalar fields in a raw JSON map.
// Skips arrays, objects, and null values (matches PHP configs.php behavior).
func writeRawMapKV(b *strings.Builder, m map[string]interface{}) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if val != "" {
				fmt.Fprintf(b, "%s=%s\n", k, val)
			}
		case float64:
			fmt.Fprintf(b, "%s=%g\n", k, val)
		case bool:
			if val {
				fmt.Fprintf(b, "%s=true\n", k)
			} else {
				fmt.Fprintf(b, "%s=false\n", k)
			}
		// Skip maps (objects), slices (arrays), and nil
		}
	}
}

func writeMapKV(b *strings.Builder, m map[string]string) {
	for k, v := range m {
		fmt.Fprintf(b, "%s=%s\n", k, v)
	}
}

func writeVarsKV(b *strings.Builder, m map[string]json.RawMessage) {
	for k, v := range m {
		s := rawToString(v)
		// Skip objects and arrays (matches PHP behavior)
		if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
			continue
		}
		fmt.Fprintf(b, "%s=%s\n", k, s)
	}
}

// SaveConfig writes configs back to ~/.captaincore/config.json.
func SaveConfig(configs FullConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return SaveConfigTo(filepath.Join(home, ".captaincore", "config.json"), configs)
}

// SaveConfigTo writes configs to a specific path with 0600 permissions.
func SaveConfigTo(path string, configs FullConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(configs, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// rawToString converts a json.RawMessage to a plain string value.
func rawToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Return as-is for non-string JSON values
	return strings.TrimSpace(string(raw))
}
