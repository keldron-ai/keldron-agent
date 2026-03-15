# K8s Simulator for S-048 Kubernetes Adapter

The Kubernetes adapter discovers GPU workloads via the Kubernetes API. Integration tests require a real cluster; we use **kind** (Kubernetes-in-Docker) or **k3d** for local dev and CI.

## Prerequisites

- Docker
- [kind](https://kind.sigs.k8s.io/) or [k3d](https://k3d.io/)

## Local Setup with kind

```bash
# Create cluster
kind create cluster --name uponline-integration

# Verify
kubectl cluster-info
kubectl get nodes
```

## Pre-create GPU Nodes and Pods

The Kubernetes adapter expects nodes with GPU capacity (e.g. `nvidia.com/gpu: 8`) and pods requesting GPUs.

### 1. Create nodes with GPU capacity

kind nodes are Docker containers. To simulate GPU nodes, patch node labels and capacity:

```bash
# Create 2 "GPU" nodes (kind uses a single node by default; for multi-node use kind config)
# For single-node kind, we patch the existing node:
kubectl label nodes kind-control-plane nvidia.com/gpu=8 --overwrite
kubectl patch node kind-control-plane -p '{"status":{"capacity":{"nvidia.com/gpu":"8"}}}'
# NOTE: The status patch is temporary — the kubelet may reconcile it away.
# Run tests immediately after patching. For persistent GPU simulation in CI,
# prefer a Kubernetes device plugin mock (e.g. github.com/NVIDIA/k8s-device-plugin).
```

For a multi-node setup, use a kind config:

```yaml
# kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
    labels:
      nvidia.com/gpu: "8"
  - role: worker
    labels:
      nvidia.com/gpu: "8"
```

```bash
kind create cluster --config kind-config.yaml --name uponline-integration
```

### 2. Create GPU pods

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod-1
  labels:
    app: training
spec:
  containers:
  - name: cuda
    image: nvidia/cuda:12.0-base
    resources:
      limits:
        nvidia.com/gpu: 2
  nodeName: kind-control-plane
---
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod-2
  labels:
    app: inference
spec:
  containers:
  - name: cuda
    image: nvidia/cuda:12.0-base
    resources:
      limits:
        nvidia.com/gpu: 4
  nodeName: kind-control-plane
EOF
```

### 3. Run integration tests

```bash
# Ensure KUBECONFIG points to kind cluster
export KUBECONFIG=~/.kube/config

# Run integration tests (K8s test will skip if cluster unavailable)
go test -tags integration ./agent/test/integration/... -v -run TestKubernetes
```

## CI Setup

In GitHub Actions or similar:

```yaml
- uses: helm/kind-action@v1.8.0
  with:
    cluster_name: uponline-integration
- run: kubectl apply -f agent/test/integration/k8s_simulator/manifests/
- run: go test -tags integration ./agent/test/integration/... -v
```

## Cleanup

```bash
kind delete cluster --name uponline-integration
```
