package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
)

func TestRegistry_CreateUnknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Create("nonexistent", config.AdapterConfig{}, nil, slog.Default())
	if err == nil {
		t.Fatal("expected error for unknown adapter, got nil")
	}
}

func TestRegistry_CreateRegistered(t *testing.T) {
	reg := NewRegistry()

	called := false
	reg.Register("test", func(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error) {
		called = true
		return nil, fmt.Errorf("test error")
	})

	_, err := reg.Create("test", config.AdapterConfig{}, nil, slog.Default())
	if !called {
		t.Fatal("constructor was not called")
	}
	if err == nil {
		t.Fatal("expected constructor error, got nil")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	reg := NewRegistry()

	ctor := func(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error) {
		return nil, nil
	}
	reg.Register("dup", ctor)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
	}()

	reg.Register("dup", ctor)
}

func TestRegistry_ConstructorError(t *testing.T) {
	reg := NewRegistry()

	reg.Register("failing", func(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error) {
		return nil, fmt.Errorf("init failed")
	})

	_, err := reg.Create("failing", config.AdapterConfig{}, nil, slog.Default())
	if err == nil {
		t.Fatal("expected error from failing constructor, got nil")
	}
}

func TestRegistry_StartAll_SkipsUnregisteredAdapter(t *testing.T) {
	reg := NewRegistry()
	reg.Register("dcgm", func(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (Adapter, error) {
		return &mockAdapter{name: "dcgm"}, nil
	})

	cfg := config.Defaults()
	cfg.Adapters["dcgm"] = config.AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	cfg.Adapters["apple_silicon"] = config.AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	holder := config.NewHolder(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	running, err := reg.StartAll(ctx, holder, slog.Default())
	if err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if len(running) != 1 {
		t.Errorf("expected 1 running adapter (dcgm), got %d", len(running))
	}
}

type mockAdapter struct {
	name string
}

func (m *mockAdapter) Name() string { return m.name }
func (m *mockAdapter) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
func (m *mockAdapter) Stop(ctx context.Context) error { return nil }
func (m *mockAdapter) Readings() <-chan RawReading {
	ch := make(chan RawReading)
	close(ch)
	return ch
}
