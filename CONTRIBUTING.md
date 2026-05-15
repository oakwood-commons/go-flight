# Contributing to go-flight

Thank you for your interest in contributing to go-flight! This document provides guidelines and best practices for contributing.

## Code of Conduct

See our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting Started

### Developer Certificate of Origin (DCO) & Commit Signing

All commits must be:
1. **GPG/SSH signed** (`-S`) — verifies commit author identity
2. **DCO signed-off** (`-s`) — certifies you have the right to submit the contribution under the project's license per the [Developer Certificate of Origin](https://developercertificate.org/)

Sign commits with both `-s` and `-S`:

```bash
git commit -s -S -m "feat: add new feature"
```

If you forget, amend the last commit:

```bash
git commit --amend -s -S --no-edit
```

### Getting Help and Support Expectations

go-flight is maintained on a best-effort, community-driven basis. We aim to:

- **Triage new issues** within 1-2 weeks
- **Review pull requests** within 2 weeks
- **Respond to security reports** within 48 hours (see [SECURITY.md](.github/SECURITY.md))

For questions and discussions, use [GitHub Discussions](https://github.com/oakwood-commons/go-flight/discussions) or open an issue.

### Prerequisites

- Go 1.26+
- golangci-lint
- Git

### Setup

```bash
# Clone the repository
git clone https://github.com/oakwood-commons/go-flight.git
cd go-flight

# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
task lint
```

## Development Workflow

### 1. Create a Branch

```bash
git checkout -b feat/my-feature
# or
git checkout -b fix/my-bugfix
```

### 2. Make Changes

Follow the coding standards below.

### 3. Test Your Changes

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./flightgroup/...
go test ./cache/...

# Run integration tests
go test ./tests/integration/... -v

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### 4. Lint Your Code

```bash
task lint
```

### 5. Commit with Conventional Commits

```bash
# Format: <type>(<scope>): <description>

git commit -m "feat(cache): add TTL support"
git commit -m "fix(flightgroup): handle nil pointer in leader election"
git commit -m "docs: update README with new examples"
git commit -m "test(cache): add concurrent access tests"
git commit -m "refactor(flightgroup): simplify callback handling"
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Tests
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `chore`: Maintenance tasks

### 6. Push and Create PR

```bash
git push origin feat/my-feature
```

Then create a Pull Request on GitHub.

## Coding Standards

### Error Handling

```go
// Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}

// Good: Use sentinel errors
var ErrNotFound = errors.New("resource not found")

// Bad: Panic in library code
if config == nil {
    panic("config is nil") // Don't do this
}
```

### Testing

Use testify for assertions:

```go
func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test"

    // Act
    result, err := MyFunction(input)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, "expected", result)
}

// Table-driven tests
func TestMyFunction_Cases(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"empty input", "", "", true},
        {"valid input", "test", "result", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := MyFunction(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Integration Tests

Integration tests live in `tests/integration/` and are designed to run locally without external dependencies.

## Project Structure

```
go-flight/
├── cache/                # Multi-tier cache manager
├── flightgroup/          # Core singleflight primitive
├── tests/
│   └── integration/      # Integration tests
├── go.mod
├── go.sum
└── README.md
```

## Documentation

- Keep README.md up to date
- Include examples for new features
- Document exported APIs with godoc comments

## Breaking Changes

This project follows semantic versioning. When making breaking changes:

1. Document in commit message: `feat!: remove deprecated API`
2. Update migration notes if needed
3. Ensure tests are updated

## Release Process

Releases are automated via GitHub Actions on tag push:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues and discussions
- For security reports, see [SECURITY.md](.github/SECURITY.md)

## Recognition

Contributors are recognized in release notes and the GitHub contributors page.

Thank you for contributing! 🎉
