package adapter

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keldron-ai/keldron-agent/internal/config"
)

// Constructor creates an adapter given its config, optional config holder for hot-reload, and a logger.
// Each adapter package registers one of these via Registry.Register.
type Constructor func(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error)

// Registry maps adapter names to their constructors.
type Registry struct {
	constructors map[string]Constructor
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		constructors: make(map[string]Constructor),
	}
}

// Register adds an adapter constructor under the given name.
// Panics if the name is already registered (indicates a programming error).
func (r *Registry) Register(name string, ctor Constructor) {
	if _, exists := r.constructors[name]; exists {
		panic(fmt.Sprintf("adapter %q already registered", name))
	}
	r.constructors[name] = ctor
}

// Create instantiates an adapter by name using its registered constructor.
func (r *Registry) Create(name string, cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error) {
	ctor, ok := r.constructors[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter: %q", name)
	}
	return ctor(cfg, holder, logger)
}

// StartAll creates and starts all enabled adapters from the config holder.
// Returns the list of running adapters (caller must stop them on shutdown).
func (r *Registry) StartAll(ctx context.Context, holder *config.Holder, logger *slog.Logger) ([]Adapter, error) {
	if holder == nil {
		return nil, fmt.Errorf("config holder is required")
	}
	cfg := holder.Get()
	if cfg == nil {
		return nil, fmt.Errorf("config holder is not initialized")
	}
	adapters := cfg.Adapters
	var running []Adapter

	for name, acfg := range adapters {
		if !acfg.Enabled {
			logger.Debug("adapter disabled, skipping", "adapter", name)
			continue
		}

		a, err := r.Create(name, acfg, holder, logger.With("adapter", name))
		if err != nil {
			// Stop already-started adapters before returning.
			for _, started := range running {
				_ = started.Stop(ctx)
			}
			return nil, fmt.Errorf("creating adapter %q: %w", name, err)
		}

		go func(a Adapter) {
			if err := a.Start(ctx); err != nil {
				logger.Error("adapter exited with error", "adapter", a.Name(), "error", err)
			}
		}(a)

		running = append(running, a)
		logger.Info("adapter started", "adapter", name)
	}

	return running, nil
}
