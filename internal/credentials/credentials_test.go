// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(tmp, ".keldron")
	if dir != wantDir {
		t.Fatalf("Dir() = %q, want %q", dir, wantDir)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(wantDir, "credentials")
	if path != wantPath {
		t.Fatalf("Path() = %q, want %q", path, wantPath)
	}
}

func TestSaveLoadDelete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	c := &Credentials{
		APIKey:    "kldn_live_abcdefghijklmnop",
		Email:     "a@b.c",
		AccountID: "acc-1",
		Endpoint:  "https://api.example.com",
	}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != c.APIKey || got.Email != c.Email || got.AccountID != c.AccountID || got.Endpoint != c.Endpoint {
		t.Fatalf("Load: %+v", got)
	}

	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}

	_, err = Load()
	if err == nil {
		t.Fatal("expected error after delete")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}
