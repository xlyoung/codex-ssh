# Contributing to codex-ssh

Thank you for your interest in contributing to codex-ssh! This document provides guidelines and instructions for contributing.

## Getting Started

### Prerequisites

- Go 1.22 or later
- Git
- An SSH client (OpenSSH)

### Setting Up the Development Environment

1. Fork the repository on GitHub

2. Clone your fork:
   ```bash
   git clone https://github.com/<your-username>/codex-ssh.git
   cd codex-ssh
   ```

3. Install dependencies:
   ```bash
   go mod download
   ```

4. Build the project:
   ```bash
   go build ./cmd/codex-ssh
   ```

5. Run the tests:
   ```bash
   go test -race ./...
   ```

## Development Workflow

1. Create a new branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes

3. Run the full test suite:
   ```bash
   go test -race ./...
   go vet ./...
   ```

4. Run the linter (install [golangci-lint](https://golangci-lint.run/usage/install/) first):
   ```bash
   golangci-lint run
   ```

5. Commit your changes with a clear, descriptive message:
   ```bash
   git commit -m "feat: add support for X"
   ```

6. Push to your fork and open a Pull Request

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` — A new feature
- `fix:` — A bug fix
- `docs:` — Documentation only changes
- `test:` — Adding or correcting tests
- `refactor:` — Code change that neither fixes a bug nor adds a feature
- `perf:` — A code change that improves performance
- `ci:` — Changes to CI configuration files
- `chore:` — Other changes that don't modify src or test files

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Write tests for new functionality
- Keep functions small and focused
- Add meaningful comments for non-obvious logic

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include a clear description of what changed and why
- Ensure all CI checks pass
- Request review from maintainers
- Be responsive to feedback

## Reporting Issues

- Use the [bug report](https://github.com/xlyoung/codex-ssh/issues/new?template=bug_report.md) template for bugs
- Use the [feature request](https://github.com/xlyoung/codex-ssh/issues/new?template=feature_request.md) template for new ideas
- Include reproduction steps for bugs

## Security

If you discover a security vulnerability, please do NOT open a public issue. Instead, please email us privately or use GitHub's security advisory feature.

## License

By contributing, you agree that your contributions will be licensed under the project's [LICENSE](LICENSE).
