package kubernetes

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// waitFor polls predicate on a short tick until it returns true or timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, msg string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

func TestIsGPUPod_WithGPU(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
						},
					},
				},
			},
		},
	}
	gpuNames := []string{"nvidia.com/gpu", "amd.com/gpu"}
	if !IsGPUPod(pod, gpuNames) {
		t.Error("expected IsGPUPod=true for pod with nvidia.com/gpu: 4")
	}
}

func TestIsGPUPod_NoGPU(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("4"),
							v1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				},
			},
		},
	}
	gpuNames := []string{"nvidia.com/gpu", "amd.com/gpu"}
	if IsGPUPod(pod, gpuNames) {
		t.Error("expected IsGPUPod=false for pod without GPU resources")
	}
}

func TestExtractGPUCount(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
						},
					},
				},
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
						},
					},
				},
			},
		},
	}
	gpuNames := []string{"nvidia.com/gpu"}
	got := ExtractGPUCount(pod, gpuNames)
	if got != 6 {
		t.Errorf("expected GPUCount=6, got %d", got)
	}
}

func TestExtractJobName_JobName(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-pod-xyz",
			Labels: map[string]string{"job-name": "my-job"},
		},
	}
	got := ExtractJobName(pod)
	if got != "my-job" {
		t.Errorf("expected job name 'my-job', got %q", got)
	}
}

func TestExtractJobName_Kubeflow(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-pod",
			Labels: map[string]string{"training.kubeflow.org/job-name": "kf-job"},
		},
	}
	got := ExtractJobName(pod)
	if got != "kf-job" {
		t.Errorf("expected job name 'kf-job', got %q", got)
	}
}

func TestExtractJobName_AppLabel(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-pod",
			Labels: map[string]string{"app": "my-app"},
		},
	}
	got := ExtractJobName(pod)
	if got != "my-app" {
		t.Errorf("expected job name 'my-app', got %q", got)
	}
}

func TestExtractJobName_Fallback(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fallback-pod-name",
		},
	}
	got := ExtractJobName(pod)
	if got != "fallback-pod-name" {
		t.Errorf("expected fallback to pod name 'fallback-pod-name', got %q", got)
	}
}

func TestExtractJobName_LabelPriority(t *testing.T) {
	// job-name should take precedence over app
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "my-pod",
			Labels: map[string]string{"job-name": "job", "app": "app"},
		},
	}
	got := ExtractJobName(pod)
	if got != "job" {
		t.Errorf("expected job-name to take precedence, got %q", got)
	}
}

func TestNodeToRackMapping(t *testing.T) {
	nodeToRack := map[string]string{
		"gpu-node-01": "rack-01",
		"gpu-node-02": "rack-01",
		"gpu-node-03": "rack-02",
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: "gpu-node-01",
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
						},
					},
				},
			},
		},
	}
	gpuNames := []string{"nvidia.com/gpu"}
	gpuPod, ok := PodToGPUPod(pod, gpuNames, nodeToRack)
	if !ok {
		t.Fatal("expected PodToGPUPod to succeed")
	}
	if gpuPod.RackID != "rack-01" {
		t.Errorf("expected RackID=rack-01, got %q", gpuPod.RackID)
	}
	if gpuPod.NodeName != "gpu-node-01" {
		t.Errorf("expected NodeName=gpu-node-01, got %q", gpuPod.NodeName)
	}
}

func TestNodeToRackMapping_Unknown(t *testing.T) {
	nodeToRack := map[string]string{
		"gpu-node-01": "rack-01",
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: "unknown-node",
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
						},
					},
				},
			},
		},
	}
	gpuNames := []string{"nvidia.com/gpu"}
	gpuPod, ok := PodToGPUPod(pod, gpuNames, nodeToRack)
	if !ok {
		t.Fatal("expected PodToGPUPod to succeed")
	}
	if gpuPod.RackID != "unknown" {
		t.Errorf("expected RackID=unknown for unmapped node, got %q", gpuPod.RackID)
	}
}

func TestBuildWorkloadState(t *testing.T) {
	pods := []GPUPod{
		{NodeName: "node-1", RackID: "rack-1", GPUCount: 4},
		{NodeName: "node-1", RackID: "rack-1", GPUCount: 2},
		{NodeName: "node-2", RackID: "rack-2", GPUCount: 8},
	}
	state := BuildWorkloadState(pods)
	if state.TotalGPUs != 14 {
		t.Errorf("expected TotalGPUs=14, got %d", state.TotalGPUs)
	}
	if state.NodeGPUMap["node-1"] != 6 {
		t.Errorf("expected NodeGPUMap[node-1]=6, got %d", state.NodeGPUMap["node-1"])
	}
	if state.NodeGPUMap["node-2"] != 8 {
		t.Errorf("expected NodeGPUMap[node-2]=8, got %d", state.NodeGPUMap["node-2"])
	}
	if state.RackGPUMap["rack-1"] != 6 {
		t.Errorf("expected RackGPUMap[rack-1]=6, got %d", state.RackGPUMap["rack-1"])
	}
	if state.RackGPUMap["rack-2"] != 8 {
		t.Errorf("expected RackGPUMap[rack-2]=8, got %d", state.RackGPUMap["rack-2"])
	}
}

func TestPodToGPUPod_NonGPUPod(t *testing.T) {
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU: resource.MustParse("8"),
						},
					},
				},
			},
		},
	}
	gpuPod, ok := PodToGPUPod(pod, []string{"nvidia.com/gpu"}, nil)
	if ok {
		t.Error("expected PodToGPUPod to return false for non-GPU pod")
	}
	if gpuPod.GPUCount != 0 {
		t.Errorf("expected zero GPUCount for non-GPU pod, got %d", gpuPod.GPUCount)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := &KubernetesConfig{}
	cfg.ApplyDefaults()
	if cfg.ResyncInterval != DefaultResyncInterval {
		t.Errorf("expected ResyncInterval=%v, got %v", DefaultResyncInterval, cfg.ResyncInterval)
	}
	if len(cfg.GPUResourceNames) != 2 {
		t.Errorf("expected 2 default GPU resource names, got %d", len(cfg.GPUResourceNames))
	}
	if cfg.NodeToRackMap == nil {
		t.Error("expected NodeToRackMap to be initialized")
	}
}

func TestAdapter_Stop_IsRunning(t *testing.T) {
	ad := newAdapter(fake.NewSimpleClientset(), KubernetesConfig{}, slog.Default())
	if ad.IsRunning() {
		t.Error("expected IsRunning=false before Start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go ad.Start(ctx)
	waitFor(t, 2*time.Second, "IsRunning=true during Start", ad.IsRunning)
	cancel()
	waitFor(t, 2*time.Second, "IsRunning=false after cancel", func() bool { return !ad.IsRunning() })
	if err := ad.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestAdapter_WithFakeClient(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := KubernetesConfig{
		ResyncInterval:   100 * time.Millisecond,
		GPUResourceNames:  []string{"nvidia.com/gpu"},
		NodeToRackMap:    map[string]string{"gpu-node-01": "rack-01"},
	}
	cfg.ApplyDefaults()

	ad := newAdapter(clientset, cfg, slog.Default())

	if ad.Name() != "kubernetes" {
		t.Errorf("expected Name=kubernetes, got %q", ad.Name())
	}

	ch := ad.Readings()
	if ch == nil {
		t.Fatal("Readings() returned nil channel")
	}

	state, ok := ad.GetWorkloadState().(WorkloadState)
	if !ok {
		t.Fatalf("GetWorkloadState did not return WorkloadState: got %T", ad.GetWorkloadState())
	}
	if state.TotalGPUs != 0 {
		t.Errorf("expected zero GPUs initially, got %d", state.TotalGPUs)
	}

	// Create a GPU pod and verify discovery
	_, err := clientset.CoreV1().Pods("default").Create(context.Background(), &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod", Namespace: "default"},
		Spec: v1.PodSpec{
			NodeName: "gpu-node-01",
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ad.Start(ctx)
	}()

	// Wait for informer to sync and discover the GPU pod
	waitFor(t, 5*time.Second, "TotalGPUs=4 after pod create", func() bool {
		s, ok := ad.GetWorkloadState().(WorkloadState)
		return ok && s.TotalGPUs == 4
	})

	state, ok = ad.GetWorkloadState().(WorkloadState)
	if !ok {
		t.Fatalf("GetWorkloadState did not return WorkloadState: got %T", ad.GetWorkloadState())
	}
	if len(state.ActivePods) != 1 {
		t.Errorf("expected 1 active pod, got %d", len(state.ActivePods))
	}
	if state.RackGPUMap["rack-01"] != 4 {
		t.Errorf("expected rack-01 to have 4 GPUs, got %d", state.RackGPUMap["rack-01"])
	}

	// Delete pod and verify state updates
	if err := clientset.CoreV1().Pods("default").Delete(context.Background(), "gpu-pod", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete pod: %v", err)
	}
	waitFor(t, 5*time.Second, "TotalGPUs=0 after pod delete", func() bool {
		s, ok := ad.GetWorkloadState().(WorkloadState)
		return ok && s.TotalGPUs == 0
	})

	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Errorf("Start returned error: %v", err)
	}

	// Stats should work after stop
	_, _, _, _, _ = ad.Stats()
}

func TestAdapter_GetWorkloadState_NilWatcher(t *testing.T) {
	ad := &KubernetesAdapter{watcher: nil}
	state, ok := ad.GetWorkloadState().(WorkloadState)
	if !ok {
		t.Fatalf("GetWorkloadState did not return WorkloadState: got %T", ad.GetWorkloadState())
	}
	if state.TotalGPUs != 0 || len(state.ActivePods) != 0 {
		t.Error("expected empty state when watcher is nil")
	}
}

func TestAdapter_Stats_NilWatcher(t *testing.T) {
	ad := &KubernetesAdapter{watcher: nil}
	pc, ec, lp, le, lea := ad.Stats()
	if pc != 0 || ec != 0 || !lp.IsZero() || le != "" || !lea.IsZero() {
		t.Error("expected zero stats when watcher is nil")
	}
}

func TestAdapter_ImplementsWorkloadAdapter(t *testing.T) {
	var _ interface {
		GetWorkloadState() interface{}
	} = (*KubernetesAdapter)(nil)
}

func TestNew_InvalidConfigDecode(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	content := `
agent:
  id: test
  log_level: info
  shutdown_timeout: 30s
adapters:
  kubernetes:
    enabled: true
    poll_interval: "10s"
    resync_interval: not-a-duration
sender:
  target: localhost:50051
buffer:
  ring_size: 100
  wal_dir: ` + tmp + `
  wal_max_size: 10MB
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	acfg := cfg.Adapters["kubernetes"]

	_, err = New(acfg, nil, slog.Default())
	if err == nil {
		t.Fatal("expected error from invalid config decode")
	}
}

func TestNew_InvalidKubeconfigPath(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	badPath := filepath.Join(tmp, "nonexistent-kubeconfig")
	content := `
agent:
  id: test
  log_level: info
  shutdown_timeout: 30s
adapters:
  kubernetes:
    enabled: true
    poll_interval: "10s"
    kubeconfig: "` + badPath + `"
sender:
  target: localhost:50051
buffer:
  ring_size: 100
  wal_dir: ` + tmp + `
  wal_max_size: 10MB
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	acfg := cfg.Adapters["kubernetes"]

	_, err = New(acfg, nil, slog.Default())
	if err == nil {
		t.Fatal("expected error from invalid kubeconfig path")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	tmp := t.TempDir()
	kubeconfig := filepath.Join(tmp, "kubeconfig")
	kubeconfigContent := `
apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: ""
`
	if err := os.WriteFile(kubeconfig, []byte(kubeconfigContent), 0600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	cfgPath := filepath.Join(tmp, "config.yaml")
	content := `
agent:
  id: test
  log_level: info
  shutdown_timeout: 30s
adapters:
  kubernetes:
    enabled: true
    poll_interval: "10s"
    kubeconfig: "` + kubeconfig + `"
    resync_interval: "5m"
sender:
  target: localhost:50051
buffer:
  ring_size: 100
  wal_dir: ` + tmp + `
  wal_max_size: 10MB
`
	if err := os.WriteFile(cfgPath, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	acfg := cfg.Adapters["kubernetes"]

	ad, err := New(acfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ad == nil {
		t.Fatal("expected non-nil adapter")
	}
	if ad.Name() != "kubernetes" {
		t.Errorf("Name: got %q", ad.Name())
	}
}
