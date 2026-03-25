// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package adapter registers hardware collectors for the Keldron agent.
//
// # External subprocesses (SEC-001)
//
// The agent avoids shell invocation; where exec.Command is used, arguments are
// fixed (no user-controlled strings passed to the shell). Call sites:
//
//   - nvidia_consumer: nvidia-smi --query-gpu=... --format=csv (Linux/Windows GPU metrics).
//   - rocm: rocm-smi --showtemp --showuse ... --json; rocm-smi --help (availability check).
//   - apple_silicon (darwin/arm64): sysctl -n hw.memsize|vm.swapusage|machdep.cpu.brand_string; vm_stat (memory/chip detection).
//   - api (darwin): sysctl -n kern.boottime (uptime for dashboard API).
//
// Prefer native libraries where available; CLI paths are configurable via YAML only.
package adapter
