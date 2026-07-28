package cliclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	credentialsDirName  = "metarrctl"
	credentialsFileName = "credentials.json"
)

// credentialsPath returns the path to the credentials file, keyed by
// server URL so multiple environments don't clobber each other's saved
// session key.
func credentialsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cliclient: locating user config dir: %w", err)
	}
	return filepath.Join(configDir, credentialsDirName, credentialsFileName), nil
}

func readCredentials() (map[string]string, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cliclient: reading credentials file: %w", err)
	}

	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("cliclient: parsing credentials file: %w", err)
	}
	return creds, nil
}

func writeCredentials(creds map[string]string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("cliclient: creating credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("cliclient: encoding credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("cliclient: writing credentials file: %w", err)
	}
	return nil
}

// SaveCredentials persists apiKey for serverURL, so future commands
// against the same server can find it without an explicit --api-key.
func SaveCredentials(serverURL, apiKey string) error {
	creds, err := readCredentials()
	if err != nil {
		return err
	}
	creds[serverURL] = apiKey
	return writeCredentials(creds)
}

// LoadCredentials returns the saved API key for serverURL, if any.
func LoadCredentials(serverURL string) (apiKey string, ok bool) {
	creds, err := readCredentials()
	if err != nil {
		return "", false
	}
	apiKey, ok = creds[serverURL]
	return apiKey, ok
}

// ClearCredentials removes the saved API key for serverURL.
func ClearCredentials(serverURL string) error {
	creds, err := readCredentials()
	if err != nil {
		return err
	}
	delete(creds, serverURL)
	return writeCredentials(creds)
}
