# Contributing to keldron-agent

PRs welcome. This guide covers how to contribute.

## Development Setup

```bash
git clone https://github.com/keldron-ai/keldron-agent
cd keldron-agent
go build ./cmd/agent
```

## Running Tests

```bash
go test ./...
```

## Code Style

- Run `go fmt ./...` before committing
- Follow standard Go conventions
- Add tests for new behavior

## Branch Naming

- `feat/` — new features
- `fix/` — bug fixes
- `docs/` — documentation only

## Pull Request Process

1. Create a branch from `main`
2. Make your changes
3. Ensure tests pass: `go test ./...`
4. Open a PR with a clear description
5. Address review feedback

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
