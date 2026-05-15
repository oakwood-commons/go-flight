# go-flight - AI Agent Instructions

## Overview
Go library providing a generic singleflight primitive with leader/follower execution model and a multi-tier cache manager.

## Key Patterns

- **Error handling**: Return errors with `fmt.Errorf("context: %w", err)`, don't panic
- **Generics**: This library is fully generic — maintain type safety throughout
- **Concurrency**: All public APIs must be safe for concurrent use
- **Testing**: Use testify (assert/require) with table-driven tests

## Conventions

- **Commits**: Use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/#specification)
- **Signing**: All commits must be GPG/SSH signed (`-S`) and include DCO sign-off (`-s`)
- **Errors**: Return errors with `fmt.Errorf("context: %w", err)`, don't panic
- **Breaking changes**: Follow semver. Document breaking changes in commit messages.

## Build & Test Commands

```bash
# Test
go test ./...                    # Run all tests

# Linting
task lint                        # Run Linter (uses pinned golangci-lint version)
task lint:fix                    # Run Linter and auto-fix issues
```

The project uses `task` (go-task/task) for builds and linting. **Always use `task lint` instead of running `golangci-lint` directly** to ensure the correct pinned version is used.

## Critical Rules

- **Test coverage**: Every new or changed file must have tests. Target 70%+ patch coverage
- **No magic values**: Always define constants or use configuration for tunable values
- **Git safety**: Never run `git commit`, `git push`, or `git commit --amend` unless the user explicitly asks
- **Concurrency safety**: All exported types must document their concurrency guarantees
- **No panics in library code**: Always return errors instead of panicking

## Project Structure

```
go-flight/
├── cache/            # Multi-tier cache manager
├── flightgroup/      # Core singleflight primitive
├── tests/
│   └── integration/  # Integration tests
├── go.mod
└── taskfile.yaml
```

## Security Scanning

```bash
task vulncheck
```
