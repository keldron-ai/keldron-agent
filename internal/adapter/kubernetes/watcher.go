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

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// K8sWatcher watches pods via the K8s API and maintains GPU workload state.
type K8sWatcher struct {
	clientset kubernetes.Interface
	config    KubernetesConfig
	logger    *slog.Logger

	mu    sync.RWMutex
	pods  map[string]GPUPod // key: namespace/name
	state WorkloadState

	syncCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastSync    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// NewK8sWatcher creates a watcher for GPU pods.
func NewK8sWatcher(clientset kubernetes.Interface, config KubernetesConfig, logger *slog.Logger) *K8sWatcher {
	w := &K8sWatcher{
		clientset: clientset,
		config:    config,
		logger:    logger,
		pods:      make(map[string]GPUPod),
	}
	w.state = BuildWorkloadState(nil)
	return w
}

// Start runs the informer and blocks until ctx is cancelled.
func (w *K8sWatcher) Start(ctx context.Context) error {
	opts := []informers.SharedInformerOption{}
	if w.config.Namespace != "" {
		opts = append(opts, informers.WithNamespace(w.config.Namespace))
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		w.config.ResyncInterval,
		opts...,
	)

	podInformer := factory.Core().V1().Pods().Informer()

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	}

	podInformer.AddEventHandler(handler)

	w.logger.Info("K8s watcher starting",
		"namespace", orAll(w.config.Namespace),
		"resync_interval", w.config.ResyncInterval,
	)

	factory.Start(ctx.Done())

	// Wait for initial sync
	if !cache.WaitForCacheSync(ctx.Done(), podInformer.HasSynced) {
		err := ctx.Err()
		if err != nil {
			w.recordError(fmt.Errorf("cache sync failed: %w", err))
		}
		return err
	}

	w.fullResyncFromCache(podInformer)
	w.recordSync()

	// Periodic full resync to catch any events missed during watch gaps
	resyncInterval := w.config.ResyncInterval
	if resyncInterval <= 0 {
		resyncInterval = DefaultResyncInterval
	}
	resyncTicker := time.NewTicker(resyncInterval)
	defer resyncTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-resyncTicker.C:
				w.fullResyncFromCache(podInformer)
				w.recordSync()
				w.logger.Debug("periodic resync completed")
			}
		}
	}()

	// Run until context cancelled
	<-ctx.Done()
	w.logger.Info("K8s watcher stopping")
	return nil
}

func orAll(s string) string {
	if s == "" {
		return "all"
	}
	return s
}

func (w *K8sWatcher) podKey(pod *v1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func gpuPodKey(p GPUPod) string {
	return p.Namespace + "/" + p.PodName
}

func (w *K8sWatcher) onAdd(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		w.recordError(fmt.Errorf("onAdd: unexpected object type %T", obj))
		return
	}
	w.upsertPod(pod)
}

func (w *K8sWatcher) onUpdate(_, newObj interface{}) {
	pod, ok := newObj.(*v1.Pod)
	if !ok {
		w.recordError(fmt.Errorf("onUpdate: unexpected object type %T", newObj))
		return
	}
	w.upsertPod(pod)
}

func (w *K8sWatcher) onDelete(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			w.recordError(fmt.Errorf("onDelete: unexpected object type %T", obj))
			return
		}
		pod, ok = tombstone.Obj.(*v1.Pod)
		if !ok {
			w.recordError(fmt.Errorf("onDelete: tombstone contained unexpected type %T", tombstone.Obj))
			return
		}
	}
	w.removePod(pod)
}

func (w *K8sWatcher) upsertPod(pod *v1.Pod) {
	gpuPod, ok := PodToGPUPod(pod, w.config.GPUResourceNames, w.config.NodeToRackMap)
	if !ok {
		// Not a GPU pod - remove if we had it (e.g. GPU request was removed)
		w.removePod(pod)
		return
	}

	key := w.podKey(pod)
	w.mu.Lock()
	w.pods[key] = gpuPod
	w.rebuildStateLocked()
	w.mu.Unlock()
}

func (w *K8sWatcher) removePod(pod *v1.Pod) {
	key := w.podKey(pod)
	w.mu.Lock()
	delete(w.pods, key)
	w.rebuildStateLocked()
	w.mu.Unlock()
}

func (w *K8sWatcher) rebuildStateLocked() {
	pods := make([]GPUPod, 0, len(w.pods))
	for _, p := range w.pods {
		pods = append(pods, p)
	}
	w.state = BuildWorkloadState(pods)
}

func (w *K8sWatcher) fullResyncFromCache(informer cache.SharedInformer) {
	lister := informer.GetStore()
	objs := lister.List()

	var pods []GPUPod
	for _, obj := range objs {
		pod, ok := obj.(*v1.Pod)
		if !ok {
			continue
		}
		gpuPod, ok := PodToGPUPod(pod, w.config.GPUResourceNames, w.config.NodeToRackMap)
		if ok {
			pods = append(pods, gpuPod)
		}
	}

	w.mu.Lock()
	w.pods = make(map[string]GPUPod, len(pods))
	for _, p := range pods {
		key := gpuPodKey(p)
		w.pods[key] = p
	}
	w.rebuildStateLocked()
	w.mu.Unlock()
}

func (w *K8sWatcher) recordSync() {
	w.syncCount.Add(1)
	w.lastSync.Store(time.Now())
}

func (w *K8sWatcher) recordError(err error) {
	w.errorCount.Add(1)
	w.lastError.Store(err.Error())
	w.lastErrorAt.Store(time.Now())
}

// Stats returns watcher sync/error counters for health reporting.
func (w *K8sWatcher) Stats() (syncCount, errorCount uint64, lastSync time.Time, lastError string, lastErrorAt time.Time) {
	syncCount = w.syncCount.Load()
	errorCount = w.errorCount.Load()
	if v := w.lastSync.Load(); v != nil {
		lastSync = v.(time.Time)
	}
	if v := w.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := w.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return syncCount, errorCount, lastSync, lastError, lastErrorAt
}

// GetState returns a deep copy of the current workload state snapshot.
func (w *K8sWatcher) GetState() WorkloadState {
	w.mu.RLock()
	defer w.mu.RUnlock()

	pods := make([]GPUPod, len(w.state.ActivePods))
	for i, p := range w.state.ActivePods {
		labels := make(map[string]string, len(p.Labels))
		for k, v := range p.Labels {
			labels[k] = v
		}
		p.Labels = labels
		pods[i] = p
	}

	nodeGPUMap := make(map[string]int, len(w.state.NodeGPUMap))
	for k, v := range w.state.NodeGPUMap {
		nodeGPUMap[k] = v
	}

	rackGPUMap := make(map[string]int, len(w.state.RackGPUMap))
	for k, v := range w.state.RackGPUMap {
		rackGPUMap[k] = v
	}

	return WorkloadState{
		Timestamp:  w.state.Timestamp,
		ActivePods: pods,
		TotalGPUs:  w.state.TotalGPUs,
		NodeGPUMap: nodeGPUMap,
		RackGPUMap: rackGPUMap,
	}
}
