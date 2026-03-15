package adapter

import (
	"fmt"
	"log/slog"
	"testing"

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
