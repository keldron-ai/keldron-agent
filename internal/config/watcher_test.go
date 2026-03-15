// Package config tests the config file watcher for hot-reload.
package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcher_FileWrite_TriggersReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := Defaults()
	initial.Agent.ID = "initial"
	holder := NewHolder(initial)

	// Use short debounce for fast test.
	watcher := NewWatcherWithDebounce(path, holder, slog.Default(), 50*time.Millisecond)

	// Write initial config so the file exists.
	if err := os.WriteFile(path, []byte(`
agent:
  id: "initial"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = watcher.Start(ctx)
	}()

	// Wait for watcher to be ready, then write new config.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`
agent:
  id: "reloaded"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write updated config: %v", err)
	}

	// Wait for debounce + reload (50ms + margin).
	time.Sleep(150 * time.Millisecond)

	if got := holder.Get().Agent.ID; got != "reloaded" {
		t.Errorf("config not reloaded: Agent.ID = %q, want %q", got, "reloaded")
	}

	cancel()
	wg.Wait()
}

func TestWatcher_RapidWrites_Debounce_OneReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := Defaults()
	initial.Agent.ID = "initial"
	holder := NewHolder(initial)

	reloadCount := atomic.Int32{}
	holder.Subscribe(func(*Config) {
		reloadCount.Add(1)
	})

	watcher := NewWatcherWithDebounce(path, holder, slog.Default(), 100*time.Millisecond)

	if err := os.WriteFile(path, []byte(`
agent:
  id: "initial"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = watcher.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// Rapid successive writes.
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte(`
agent:
  id: "rapid"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire (only once).
	time.Sleep(200 * time.Millisecond)

	count := reloadCount.Load()
	if count != 1 {
		t.Errorf("expected 1 reload from rapid writes, got %d", count)
	}
	if holder.Get().Agent.ID != "rapid" {
		t.Errorf("Agent.ID = %q, want %q", holder.Get().Agent.ID, "rapid")
	}

	cancel()
	wg.Wait()
}

func TestWatcher_InvalidYAML_KeepsOldConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := Defaults()
	initial.Agent.ID = "valid"
	holder := NewHolder(initial)

	watcher := NewWatcherWithDebounce(path, holder, slog.Default(), 50*time.Millisecond)

	if err := os.WriteFile(path, []byte(`
agent:
  id: "valid"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = watcher.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// Write invalid YAML.
	if err := os.WriteFile(path, []byte(`{{{ invalid yaml`), 0644); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if got := holder.Get().Agent.ID; got != "valid" {
		t.Errorf("config changed despite invalid YAML: Agent.ID = %q, want %q", got, "valid")
	}

	cancel()
	wg.Wait()
}

func TestWatcher_VimStyleWrite_Detected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := Defaults()
	initial.Agent.ID = "before"
	holder := NewHolder(initial)

	watcher := NewWatcherWithDebounce(path, holder, slog.Default(), 50*time.Millisecond)

	if err := os.WriteFile(path, []byte(`
agent:
  id: "before"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = watcher.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)

	// Simulate vim: remove file, then create new one (atomic rename would trigger CREATE).
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`
agent:
  id: "after_vim"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write after remove: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if got := holder.Get().Agent.ID; got != "after_vim" {
		t.Errorf("config not reloaded after vim-style write: Agent.ID = %q, want %q", got, "after_vim")
	}

	cancel()
	wg.Wait()
}

func TestWatcher_ContextCancel_StopsCleanly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	initial := Defaults()
	initial.Agent.ID = "test"
	holder := NewHolder(initial)

	watcher := NewWatcherWithDebounce(path, holder, slog.Default(), 2*time.Second)

	if err := os.WriteFile(path, []byte(`
agent:
  id: "test"
  log_level: "info"
  shutdown_timeout: "10s"
sender:
  target: "localhost:443"
buffer:
  ring_size: 1000
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- watcher.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err != nil {
		t.Errorf("watcher Start returned error after cancel: %v", err)
	}
}
