# Contributing to Keldron Agent

Thanks for your interest in contributing to the Keldron Agent! This guide will help you get started.

## Quick Links

- **Issues:** [github.com/keldron-ai/keldron-agent/issues](https://github.com/keldron-ai/keldron-agent/issues)
- **Docs:** [keldron.ai/docs](https://keldron.ai/docs)
- **Community:** [github.com/keldron-ai/keldron-agent/discussions](https://github.com/keldron-ai/keldron-agent/discussions)

## How to Contribute

### Reporting Bugs

Open an issue with:
- Your OS and hardware (e.g., "macOS 15.3, M4 Pro" or "Ubuntu 24.04, RTX 4090")
- Agent version (`keldron-agent --version`)
- What you expected vs what happened
- Relevant log output (run with `--log-level debug` for verbose logging)

### Suggesting Features

Open an issue with the `enhancement` label. Describe the use case — what problem does this solve for you? We prioritize features based on community demand.

### Submitting Code

1. Fork the repo and create a branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```

2. Make your changes. Follow the conventions below.

3. Test your changes:
   ```bash
   go test ./... -race
   go vet ./...
   ```

4. Commit using conventional commits:
   ```
   feat(adapter): add Intel Arc GPU support
   fix(scoring): correct TDP lookup for RTX 5090
   docs: update configuration reference
   ```

5. Open a pull request against `main`. Describe what changed and why.

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+ (for the embedded dashboard frontend)
- Make

### Building

```bash
# Full build (frontend + agent binary)
make build

# Agent only (skip frontend)
go build -o keldron-agent ./cmd/agent

# Run in dev mode
./dev.sh

# Run tests
make test
```

### Project Structure

```
cmd/agent/          Entry point — CLI dispatch and agent startup
internal/
  adapters/         Hardware adapters (Apple Silicon, NVIDIA, Linux thermal)
  api/              HTTP API + embedded frontend server
  cloud/            Cloud streaming client
  config/           YAML config loading + env overrides
  credentials/      ~/.keldron/credentials file management
  health/           Health computation engine
  login/            CLI login command
  logout/           CLI logout command
  normalizer/       Telemetry normalization + device registry
  scan/             CLI scan command
  scoring/          Layer 1 risk scoring engine
  whoami/           CLI whoami command
frontend/           Embedded React dashboard (Vite + TypeScript)
```

## Conventions

### Code Style

- Go: standard `gofmt` formatting. Run `go vet ./...` before committing.
- TypeScript: Prettier defaults. Run `npm run lint` in `frontend/`.
- No external linter config needed — just use the defaults.

### Branch Naming

```
feat/short-description    — new features
fix/short-description     — bug fixes
docs/short-description    — documentation only
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(adapter): add Raspberry Pi thermal adapter
fix(scoring): handle NaN in volatility sub-score
docs: add Linux quick-start to README
test(scoring): add edge case for zero utilization
chore: update Go dependencies
```

### Adding a New Adapter

Adapters are the most common contribution. To add support for new hardware:

1. Create a new file in `internal/adapters/` (e.g., `intel_arc.go`)
2. Implement the `Adapter` interface:
   ```go
   type Adapter interface {
       Name() string
       Poll(ctx context.Context) ([]RawReading, error)
       Start(ctx context.Context) error
       Stop() error
   }
   ```
3. Add device entries to the spec registry in `internal/normalizer/`
4. Register the adapter in the config auto-detection logic
5. Add a test file (e.g., `intel_arc_test.go`)
6. Update the README with the new supported hardware

### Adding to the Device Spec Registry

The registry maps hardware models to thermal limits, TDP, and behavior classes. To add a new device:

1. Find the file in `internal/normalizer/` that contains the registry entries
2. Add an entry with the model name, thermal throttle temperature, TDP, and behavior class (`datacenter`, `consumer_active_cooled`, or `soc_integrated`)
3. Use manufacturer specifications — do not guess thermal limits

## What We're Looking For

High-impact contributions:
- **New adapters** — Raspberry Pi, Intel Arc, Jetson, AMD consumer GPUs
- **Bug fixes** — especially around edge cases in sensor readings
- **Documentation** — quick-start guides, configuration examples, troubleshooting
- **Testing** — unit tests, integration tests, soak test improvements
- **Docker improvements** — multi-arch builds, smaller image size

## What We're Not Accepting Right Now

- Changes to the risk scoring formula (this is published and must remain deterministic)
- Cloud platform features (the cloud is proprietary, not open source)
- Major architectural changes without prior discussion in an issue

## Code of Conduct

Be kind. Be constructive. We're building something useful together.

Harassment, discrimination, and personal attacks will not be tolerated. If you experience or witness unacceptable behavior, contact ransom@keldron.ai.

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

Open a discussion on GitHub or email ransom@keldron.ai. We're happy to help you get started.
