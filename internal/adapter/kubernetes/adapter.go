// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const channelBuffer = 16

// KubernetesAdapter discovers GPU-scheduled pods via the K8s API.
type KubernetesAdapter struct {
	k8sCfg   KubernetesConfig
	watcher  *K8sWatcher
	readings chan adapter.RawReading
	logger   *slog.Logger

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	running   atomic.Bool
}

// New creates a KubernetesAdapter from the adapter config.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var k8sCfg KubernetesConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&k8sCfg); err != nil {
			return nil, fmt.Errorf("decoding kubernetes config: %w", err)
		}
	}
	k8sCfg.ApplyDefaults()

	restConfig, err := buildRestConfig(k8sCfg.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("building K8s config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating K8s clientset: %w", err)
	}

	return newAdapter(clientset, k8sCfg, logger), nil
}

// newAdapter creates the adapter with an injected clientset (used by New and tests).
func newAdapter(clientset kubernetes.Interface, k8sCfg KubernetesConfig, logger *slog.Logger) *KubernetesAdapter {
	k8sCfg.ApplyDefaults()
	watcher := NewK8sWatcher(clientset, k8sCfg, logger)

	return &KubernetesAdapter{
		k8sCfg:   k8sCfg,
		watcher:  watcher,
		readings: make(chan adapter.RawReading, channelBuffer),
		logger:   logger,
	}
}

func buildRestConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// Name returns the adapter identifier.
func (k *KubernetesAdapter) Name() string { return "kubernetes" }

// Readings returns a channel of raw readings. For the K8s adapter, this channel
// receives nothing (workload state is not telemetry). Use GetWorkloadState() for data.
func (k *KubernetesAdapter) Readings() <-chan adapter.RawReading {
	return k.readings
}

// GetWorkloadState returns the current GPU workload state (implements WorkloadAdapter).
func (k *KubernetesAdapter) GetWorkloadState() interface{} {
	if k.watcher == nil {
		return WorkloadState{}
	}
	return k.watcher.GetState()
}

// Start begins the watcher. Blocks until ctx is cancelled or Stop is called.
func (k *KubernetesAdapter) Start(ctx context.Context) error {
	k.mu.Lock()
	if k.running.Load() {
		k.mu.Unlock()
		return fmt.Errorf("adapter already started")
	}
	ctx, cancel := context.WithCancel(ctx)
	k.cancel = cancel
	k.done = make(chan struct{})
	localDone := k.done
	k.running.Store(true)
	k.mu.Unlock()

	defer func() {
		k.running.Store(false)
		close(localDone)
	}()

	k.logger.Info("Kubernetes adapter starting",
		"namespace", orEmpty(k.k8sCfg.Namespace, "all"),
		"resync_interval", k.k8sCfg.ResyncInterval,
	)

	err := k.watcher.Start(ctx)

	k.closeOnce.Do(func() {
		close(k.readings)
	})

	return err
}

func orEmpty(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}

// Stop gracefully shuts down the adapter by cancelling the watcher context
// and waiting for the Start goroutine to finish.
func (k *KubernetesAdapter) Stop(ctx context.Context) error {
	k.logger.Info("Kubernetes adapter shutting down")
	k.mu.Lock()
	cancel := k.cancel
	done := k.done
	k.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRunning returns true if the adapter's Start loop is active (for health.AdapterProvider).
func (k *KubernetesAdapter) IsRunning() bool {
	return k.running.Load()
}

// Stats returns sync count, error count, last sync time, last error for health reporting.
func (k *KubernetesAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	if k.watcher == nil {
		return 0, 0, time.Time{}, "", time.Time{}
	}
	return k.watcher.Stats()
}
