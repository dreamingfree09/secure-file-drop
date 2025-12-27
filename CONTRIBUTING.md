# Contributing to Secure File Drop

Thank you for considering contributing to Secure File Drop! This document provides guidelines and instructions for contributors.

## Getting Started

### Prerequisites

- Go 1.24 or later
- Docker and Docker Compose
- GCC (for building the native C hash utility)
- OpenSSL development headers (`libssl-dev` or `openssl-dev`)
- Git

### Local Development Setup

1. **Clone the repository:**
   ```bash
   git clone <your-repo-url>
   cd secure-file-drop
   ```

2. **Create your `.env` file:**
   ```bash
   cp .env.example .env
   ```

3. **Generate secrets:**
   ```bash
   # Session and download secrets
   openssl rand -hex 32  # Use for SFD_SESSION_SECRET
   openssl rand -hex 32  # Use for SFD_DOWNLOAD_SECRET
   
   # Passwords
   openssl rand -base64 24  # Use for SFD_ADMIN_PASS
   openssl rand -base64 24  # Use for POSTGRES_PASSWORD
   openssl rand -base64 24  # Use for MINIO_ROOT_PASSWORD
   ```
   Update `.env` with the generated values.

4. **Start services:**
   ```bash
   docker-compose up -d
   ```

5. **Build the backend:**
   ```bash
   go build ./cmd/backend
   ```

6. **Build the native hash utility:**
   ```bash
   cd native
   make
   cd ..
   ```

## Development Workflow

### Running Tests

Run all tests:
```bash
go test ./...
```

Run tests with coverage:
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Run specific tests:
```bash
go test ./internal/server -v -run TestAuth
```

Run end-to-end tests:
```bash
go test ./tests/e2e -v
```

### Code Quality

#### Formatting
All Go code must be formatted with `gofmt`:
```bash
gofmt -w .
```

#### Linting
We use `golangci-lint` with a strict configuration:
```bash
golangci-lint run ./...
```

Fix auto-fixable issues:
```bash
golangci-lint run --fix ./...
```

#### Static Analysis
Run `go vet` before committing:
```bash
go vet ./...
```

### Building the Native Utility

```bash
cd native
make clean
make
make test
```

## Code Style Guidelines

### Go Code
- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` for formatting
- Keep functions small and focused
- Write tests for new features
- Document exported functions and types
- Use meaningful variable names
- Avoid global mutable state

### C Code
- Follow POSIX/ISO C standards
- Keep the utility minimal and auditable
- Check all return values
- Free all allocated memory
- Document non-obvious logic

### Commit Messages
Follow the [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `test:` Adding or updating tests
- `refactor:` Code refactoring
- `chore:` Tooling, dependencies, etc.
- `perf:` Performance improvements

**Examples:**
```
feat(upload): add size limit enforcement

Implemented server-side upload size validation using http.MaxBytesReader
to prevent resource exhaustion attacks.

Closes #42
```

```
fix(auth): prevent timing attack in password comparison

Replaced string comparison with constant-time comparison function
to mitigate timing-based password enumeration.
```

## Testing Guidelines

### Unit Tests
- Test edge cases and error paths
- Use table-driven tests where appropriate
- Mock external dependencies (DB, MinIO) when possible
- Aim for >60% coverage on critical paths

### Integration Tests
- Use `tests/e2e` for full-stack integration tests
- Test the complete upload → hash → download flow
- Verify error handling end-to-end

### Test Naming
```go
func TestFunctionName_Scenario(t *testing.T) { ... }
```

Examples:
- `TestUploadHandler_Success`
- `TestUploadHandler_SizeLimitExceeded`
- `TestVerifyToken_Expired`

## Pull Request Process

1. **Create a feature branch:**
   ```bash
   git checkout -b feat/your-feature-name
   ```

2. **Make your changes:**
   - Write code
   - Add tests
   - Update documentation if needed

3. **Ensure all checks pass:**
   ```bash
   go test ./...
   go vet ./...
   golangci-lint run ./...
   gofmt -w .
   ```

4. **Commit your changes:**
   ```bash
   git add .
   git commit -m "feat(scope): description"
   ```

5. **Push and create a PR:**
   ```bash
   git push origin feat/your-feature-name
   ```
   Open a pull request on GitHub with a clear description.

6. **Address review feedback:**
   - Make requested changes
   - Push updates to the same branch
   - Re-request review when ready

### PR Requirements
- All tests must pass
- Code must be formatted (`gofmt`)
- Linter must pass (`golangci-lint`)
- New features must include tests
- Breaking changes must be documented
- Commit messages must follow conventions

## Project Structure

```
.
├── cmd/
│   └── backend/          # Main application entry point
├── internal/
│   ├── db/               # Database migrations and utilities
│   └── server/           # HTTP server, handlers, middleware
├── native/               # C hash utility
├── tests/
│   └── e2e/              # End-to-end integration tests
├── web/
│   └── static/           # Frontend assets
├── docs/                 # Documentation
└── journal/              # Development log
```

## Security

### Reporting Vulnerabilities
Please **do not** open public issues for security vulnerabilities. Instead:
1. Email the maintainer with details
2. Allow time for a fix before public disclosure

### Security Guidelines
- Never commit secrets to version control
- Use `.env` for local secrets (gitignored)
- Validate all user inputs
- Use constant-time comparisons for secrets
- Follow OWASP guidelines for web security

## Documentation

When adding features:
- Update [docs/SPEC.md](docs/SPEC.md) if architecture changes
- Update [docs/TRACKER.md](docs/TRACKER.md) for milestone progress
- Add entries to [journal/DEVLOG.md](journal/DEVLOG.md) for significant changes
- Update the [README.md](README.md) if user-facing changes occur

## Questions?

- Check existing issues and PRs first
- Open a new issue for questions or discussions
- Tag maintainers if urgent

## License

By contributing, you agree that your contributions will be licensed under the MIT License (see [LICENSE](LICENSE)).

---

Thank you for contributing! 🎉
