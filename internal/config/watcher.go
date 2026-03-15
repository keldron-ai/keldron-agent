// Package config provides a file watcher for hot-reloading the agent config.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

const defaultDebounce = 2 * time.Second

// Watcher monitors a config file for changes and triggers reload.
type Watcher struct {
	path     string
	filename string
	holder   *Holder
	logger   *slog.Logger
	debounce time.Duration

	reloadCount  atomic.Uint64
	lastReloadAt atomic.Value // time.Time
	lastError    atomic.Value // string
}

// NewWatcher creates a Watcher for the given config path.
func NewWatcher(path string, holder *Holder, logger *slog.Logger) *Watcher {
	return NewWatcherWithDebounce(path, holder, logger, defaultDebounce)
}

// NewWatcherWithDebounce creates a Watcher with a custom debounce duration (for testing).
func NewWatcherWithDebounce(path string, holder *Holder, logger *slog.Logger, debounce time.Duration) *Watcher {
	return &Watcher{
		path:     path,
		filename: filepath.Base(path),
		holder:   holder,
		logger:   logger,
		debounce: debounce,
	}
}

// Start begins watching the config file. Blocks until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	if w.holder == nil {
		return fmt.Errorf("watcher holder is required")
	}
	if w.logger == nil {
		return fmt.Errorf("watcher logger is required")
	}
	watchDir := filepath.Dir(w.path)
	if watchDir == "." {
		watchDir = "./"
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := watcher.Add(watchDir); err != nil {
		return err
	}

	w.debounceReload(ctx, watcher.Events, watcher.Errors)
	return nil
}

func (w *Watcher) debounceReload(ctx context.Context, events <-chan fsnotify.Event, errs <-chan error) {
	var timer *time.Timer
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			// Match our config file (path may differ if watchDir is different)
			if filepath.Base(event.Name) != w.filename {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounce, func() {
				w.reload()
			})
		case err, ok := <-errs:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", "error", err)
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

func (w *Watcher) reload() {
	newCfg, err := Load(w.path)
	if err != nil {
		w.lastError.Store(err.Error())
		w.logger.Error("failed to parse updated config", "error", err)
		return
	}
	oldCfg := w.holder.Get()
	if err := w.holder.Update(newCfg); err != nil {
		w.lastError.Store(err.Error())
		w.logger.Error("config update rejected", "error", err)
		return
	}
	w.reloadCount.Add(1)
	w.lastReloadAt.Store(time.Now())
	w.lastError.Store("") // clear on success

	if oldCfg == nil {
		w.logger.Info("config reloaded", "change", "initial_load")
		return
	}
	w.logChanges(oldCfg, newCfg)
}

func (w *Watcher) logChanges(old, new *Config) {
	// Log hot-reloadable changes
	if old.Agent.LogLevel != new.Agent.LogLevel {
		w.logger.Info("config reloaded",
			"change", "agent.log_level",
			"old", old.Agent.LogLevel,
			"new", new.Agent.LogLevel,
		)
	}
	for name, newAcfg := range new.Adapters {
		oldAcfg, ok := old.Adapters[name]
		if !ok {
			w.logger.Warn("config changed for adapters."+name+" (added) but requires agent restart to take effect",
				"new_enabled", newAcfg.Enabled,
			)
			continue
		}
		if oldAcfg.PollInterval != newAcfg.PollInterval {
			w.logger.Info("config reloaded",
				"change", "adapters."+name+".poll_interval",
				"old", oldAcfg.PollInterval,
				"new", newAcfg.PollInterval,
			)
		}
	}
	for name := range old.Adapters {
		if _, ok := new.Adapters[name]; !ok {
			w.logger.Warn("config changed for adapters." + name + " (removed) but requires agent restart to take effect")
		}
	}
	// Rack mapping comparison (simple: log if different)
	if !mapsEqual(old.RackMapping, new.RackMapping) {
		w.logger.Info("config reloaded", "change", "rack_mapping")
	}

	// Log non-reloadable changes at warn
	if old.Sender.Target != new.Sender.Target {
		w.logger.Warn("config changed for sender.target but requires agent restart to take effect",
			"old", old.Sender.Target,
			"new", new.Sender.Target,
		)
	}
	if old.Buffer.RingSize != new.Buffer.RingSize {
		w.logger.Warn("config changed for buffer.ring_size but requires agent restart to take effect",
			"old", old.Buffer.RingSize,
			"new", new.Buffer.RingSize,
		)
	}
	if old.Health.Bind != new.Health.Bind {
		w.logger.Warn("config changed for health.bind but requires agent restart to take effect",
			"old", old.Health.Bind,
			"new", new.Health.Bind,
		)
	}
	for name, newAcfg := range new.Adapters {
		oldAcfg, ok := old.Adapters[name]
		if !ok {
			// Already logged as added in the hot-reloadable block above
			continue
		}
		if oldAcfg.Enabled != newAcfg.Enabled {
			w.logger.Warn("config changed for adapters."+name+".enabled but requires agent restart to take effect",
				"old", oldAcfg.Enabled,
				"new", newAcfg.Enabled,
			)
		}
		if oldAcfg.Endpoint != newAcfg.Endpoint {
			w.logger.Warn("config changed for adapters."+name+".endpoint but requires agent restart to take effect",
				"old", oldAcfg.Endpoint,
				"new", newAcfg.Endpoint,
			)
		}
	}
}

// Path returns the config file path being watched.
func (w *Watcher) Path() string {
	return w.path
}

// ReloadStats returns reload count, last reload time, and last error for health reporting.
func (w *Watcher) ReloadStats() (count uint64, lastReloadAt time.Time, lastError string) {
	count = w.reloadCount.Load()
	if v := w.lastReloadAt.Load(); v != nil {
		lastReloadAt = v.(time.Time)
	}
	if v := w.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	return count, lastReloadAt, lastError
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
