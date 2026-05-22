# Contributing to ChartScan

Thanks for taking the time to contribute. This document walks you through everything you need to develop, test, and submit changes.

## Prerequisites

- **Go 1.26** or newer (declared in [`go.mod`](go.mod)).
- **Helm** on your `PATH`. ChartScan shells out to `helm lint`, `helm template`, and `helm dependency update` and the test suite assumes Helm is available.
- **Git**, for cloning and for ChartScan's auto-discovery feature.

## Repository layout

```
chartscan/
├── cmd/chartscan/        # CLI entry point — Cobra commands wired in main.go.
├── internal/
│   ├── finder/           # Recursive discovery of Helm charts via Chart.yaml.
│   ├── models/           # Result, Config, TestSuite data structures.
│   └── renderer/         # Linting, templating, value-reference checking.
├── pkg/utils/            # Shared utilities (logger).
├── mock/                 # Sample charts (valid + invalid) used by tests and smoke runs.
├── demo/                 # Demo chart and the README gif.
├── docs/                 # User documentation (usage, configuration).
└── .github/workflows/    # CI: go-test on every PR, go-build on release.
```

## Local development

Install dependencies and run the CLI:

```bash
go mod tidy
go run ./cmd/chartscan scan mock/charts
```

Build a binary:

```bash
go build -o chartscan ./cmd/chartscan
./chartscan version
```

## Testing

Run the full unit test suite:

```bash
go test ./...
```

The repo also ships a [`test.sh`](test.sh) helper that runs `go fmt`, `go vet`, `go test`, and a smoke `scan` against `mock/charts`. Note that `test.sh` currently hard-codes a developer-specific `GOROOT` near the top of the file — adjust the `export` lines for your machine, or just run the underlying commands directly:

```bash
go fmt ./...
go vet ./...
go test ./...
go run ./cmd/chartscan scan mock/charts
```

## Continuous integration

Two GitHub Actions workflows live in [`.github/workflows/`](.github/workflows):

- **`go-test.yml`** — runs `go vet` and `go test -v ./...` on every pull request and push to `main`. Uses Go 1.26.
- **`go-build.yml`** — runs when a release is created. Builds `linux/amd64`, `linux/arm64`, and `linux/386` binaries with the release tag injected as the version, and attaches them as release assets.

Your PR must pass `go-test` before it can be merged.

## Submitting a pull request

1. Fork the repository.
2. Create a feature branch: `git checkout -b my-feature`.
3. Make your changes. Keep commits focused and write descriptive messages.
4. Run `go fmt ./... && go vet ./... && go test ./...` and make sure everything passes.
5. Update the user-facing docs in [`docs/`](docs) or [`README.md`](README.md) if you changed CLI behavior, flags, or configuration syntax.
6. Push the branch: `git push origin my-feature`.
7. Open a pull request against `main` and describe what changed and why.

## Reporting bugs

Open an issue on [GitHub](https://github.com/Jaydee94/chartscan/issues) with:

- The ChartScan version (`chartscan version`) and Helm version (`helm version`).
- The exact command you ran.
- The full output, plus the chart or a minimal reproducer if possible.
