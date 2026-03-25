// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package credentials reads and writes ~/.keldron/credentials for CLI login.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Validate checks that required fields are present and non-empty.
func (c *Credentials) Validate() error {
	if c.APIKey == "" {
		return errors.New("credentials: api_key is required")
	}
	if c.Email == "" {
		return errors.New("credentials: email is required")
	}
	if c.AccountID == "" {
		return errors.New("credentials: account_id is required")
	}
	return nil
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
	if err := creds.Validate(); err != nil {
		return nil, fmt.Errorf("credentials file is incomplete: %w", err)
	}
	return &creds, nil
}

// Save writes credentials with restricted permissions.
func Save(creds *Credentials) error {
	if creds == nil {
		return errors.New("credentials: cannot save nil credentials")
	}
	if err := creds.Validate(); err != nil {
		return fmt.Errorf("refusing to save invalid credentials: %w", err)
	}

	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Enforce directory permissions in case it already existed with wider perms.
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file then rename.
	tmp, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
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
