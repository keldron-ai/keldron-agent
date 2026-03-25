# Changelog

## v0.1.0 — March 2026

First public release of the Keldron Agent — vendor-neutral hardware monitoring with risk intelligence.

### Agent

- **Apple Silicon adapter** — native IOKit bindings for M1–M5 Macs. GPU/SoC temperature, power draw, utilization, Neural Engine utilization, memory pressure, and thermal state. No sudo required.
- **NVIDIA consumer adapter** — NVML-based monitoring for RTX 3090/4090/5090 and other consumer GPUs. Temperature, power, utilization, throttle state, clock speed.
- **NVIDIA datacenter adapter** — DCGM-based monitoring for H100, B200, and other datacenter GPUs.
- **AMD adapter** — ROCm SMI-based monitoring for MI300X, RX 7900 XTX, and other AMD GPUs.
- **Linux thermal adapter** — generic sysfs/hwmon support for CPU temperature, fan RPM, and thermal zone readings on any Linux system.
- **Layer 1 risk scoring engine** — composite 0–100 risk score from four sub-scores: thermal margin, power headroom, load volatility, and memory pressure. Behavior-class-aware thresholds (datacenter, consumer, SoC) prevent false alarms across different hardware types.
- **Device spec registry** — 15+ hardware entries with per-device thermal limits, TDP, and behavior classification.
- **Embedded web dashboard** — single-device monitoring at `localhost:9200`. Risk hex badge, metric sparklines, drill-down charts, active processes table. Dark theme with kinetic elements.
- **Cloud streaming** — stream telemetry to Keldron Cloud with 2-second default interval. Automatic buffering and retry on network failure.
- **CLI commands** — `scan` (one-shot status), `login` (email/password or API key), `logout`, `whoami`.
- **Prometheus output** — full metrics at `/metrics` on port 9100.
- **YAML configuration** — single config file with adapter auto-detection, environment variable overrides, and credentials file fallback.
- **OpenClaw skill** — dual-mode skill (local + cloud) with auto-setup, fleet queries, and proactive monitoring.

### Security

- All HTTP servers bind to `127.0.0.1` by default (configurable for LAN access).
- Cloud telemetry over TLS 1.2+ (HTTPS and gRPC).
- Credentials stored with 0600 file permissions.
- Agent is strictly read-only — no system modifications, no unsolicited external connections. Local HTTP servers bind to `127.0.0.1` by default.
- Pre-launch security audit passed (SEC-001).
