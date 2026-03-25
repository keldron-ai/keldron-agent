// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package credentials reads and writes ~/.keldron/credentials for CLI login.
package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Credentials holds persisted Keldron Cloud authentication data.
type Credentials struct {
	APIKey    string `json:"api_key"`
	Email     string `json:"email"`
	AccountID string `json:"account_id"`
	Endpoint  string `json:"endpoint,omitempty"` // defaults to https://api.keldron.ai
}

// Dir returns ~/.keldron.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".keldron"), nil
}

// Path returns ~/.keldron/credentials.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials"), nil
}

// Load reads credentials from disk. Returns an error if the file is missing or invalid.
func Load() (*Credentials, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// Save writes credentials with restricted permissions.
func Save(creds *Credentials) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "credentials")
	return os.WriteFile(path, data, 0o600)
}

// Delete removes the credentials file. Missing file is not an error.
func Delete() error {
	path, err := Path()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
