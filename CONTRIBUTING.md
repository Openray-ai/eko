# Contributing to Ekō

Thanks for your interest in contributing. Ekō is an open-source, context-aware security proxy for AI applications, with first-class detectors for African fintech and banking compliance. Contributions of all sizes are welcome, from a single new pattern to a full provider integration.

## Where help is most needed

- **New detection patterns** for African contexts (formats, regulations, institutions across Nigeria, Kenya, South Africa, Ghana, and beyond)
- **Provider integrations** (Anthropic, Google, Cohere, Mistral, local LLMs)
- **Tests** — edge cases, false-positive reduction, performance benchmarks
- **Documentation** — tutorials, deployment guides, translations

## Reporting issues

- **Bugs and feature requests** → open a GitHub issue with the relevant template
- **Security vulnerabilities** → see [SECURITY.md](SECURITY.md). Do **not** file public issues for security problems.

## Development setup

Requirements:

- Go 1.24 or newer
- Docker (optional, for the SLM sidecar and integration tests)
- Redis (optional, for token-vault features)

Get the code and run the basics:

```bash
git clone https://github.com/Openray-ai/eko.git
cd eko
make install        # go mod download && tidy
make build          # builds bin/eko
make run-config     # runs against configs/config.example.yaml
```

## Running tests

```bash
make test            # unit tests
make test-coverage   # unit tests with HTML coverage report
make test-integration # requires server running on localhost:8080
make test-all        # both
```

Benchmarks:

```bash
make bench           # baseline benchmarks
make bench-compare   # compare against saved baseline
make bench-profile-cpu
make bench-profile-mem
```

All tests must pass before a PR will be merged. New behavior should come with tests.

## Code style

- Format with `make fmt` (`gofmt`)
- Lint with `make lint` (`go vet` + gofmt check)
- Keep functions focused; prefer small packages over large ones
- Follow the patterns already used in `internal/` and `pkg/` rather than introducing new conventions

## Adding a new detection pattern

Most contributions will land here.

1. Open `patterns/` and pick or create the appropriate YAML file (e.g. `patterns/african.yaml` for region-specific patterns).
2. Add an entry with: `name`, `pattern` (regex), `severity`, `description`, and a tag indicating jurisdiction or category.
3. Add positive and negative test cases in the corresponding `_test.go` file under `internal/detector/`.
4. Run `make test` and confirm both your new cases and the full suite pass.
5. If the pattern is high-risk for false positives, add a Luhn check or context-aware validator following the credit-card pattern as a reference.

## Pull request process

1. Fork the repo and create a feature branch from `develop`: `feature/<short-name>` or `fix/<short-name>`
2. Make focused commits with clear messages. Match the existing commit style (`Add ...`, `Fix ...`, `Address PR #N review feedback`)
3. Run `make test` and `make lint` before pushing
4. Open a PR against `develop`. Describe **what** changed and **why**, and include test evidence
5. Address review feedback in additional commits rather than force-pushing once review has started

## Licensing

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE) that covers the project.
