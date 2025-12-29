# Integration Tests

This directory contains API integration tests for the Secure File Drop application. These tests verify the complete functionality of the system by testing real HTTP endpoints with live dependencies.

## Overview

Integration tests validate:
- Complete user workflows (register → login → upload → download → delete)
- API endpoint responses and error handling
- Database operations and data persistence
- MinIO object storage integration
- Session management and authentication
- Admin operations and metrics

## Prerequisites

### Required Services

Integration tests require the following services to be running:

```bash
# Start all services with Docker Compose
docker-compose up -d postgres minio

# Verify services are healthy
docker-compose ps
```

### Environment Variables

Set these environment variables before running tests:

```bash
export SFD_DB_HOST=localhost
export SFD_DB_PORT=5432
export SFD_DB_USER=postgres
export SFD_DB_PASSWORD=postgres
export SFD_DB_NAME=sfd_test
export SFD_MINIO_ENDPOINT=localhost:9000
export SFD_MINIO_ACCESS_KEY=minioadmin
export SFD_MINIO_SECRET_KEY=minioadmin
export SFD_MINIO_BUCKET=sfd-test
export SFD_SESSION_SECRET=test-session-secret-min-32-chars-long
export SFD_DOWNLOAD_SECRET=test-download-secret-min-32-chars
export SFD_ADMIN_USER=admin
export SFD_ADMIN_PASS_HASH='$2a$10$test.hash.here'
```

### Database Setup

Integration tests need a clean database schema:

```bash
# Create test database
psql -U postgres -h localhost -c "CREATE DATABASE sfd_test;"

# Apply schema
psql -U postgres -h localhost -d sfd_test -f internal/db/schema.sql
```

## Running Tests

### Run All Integration Tests

```bash
# With verbose output
go test -v -tags=integration ./tests/integration/...

# With coverage
go test -v -tags=integration -coverprofile=coverage.out ./tests/integration/...
go tool cover -html=coverage.out
```

### Run Specific Test

```bash
go test -v -tags=integration -run TestAPIWorkflow ./tests/integration/
```

### Run in CI

Integration tests are automatically run in CI when you push to the repository. The GitHub Actions workflow:

1. Starts PostgreSQL and MinIO services
2. Applies database schema
3. Runs integration tests with coverage
4. Uploads coverage reports

See [.github/workflows/ci.yml](../../.github/workflows/ci.yml) for details.

## Test Structure

### api_test.go

Main integration test file containing:

- **TestAPIWorkflow**: Complete user workflow test covering:
  - Health check endpoint
  - User registration
  - Admin login and session management
  - Quota retrieval
  - File metadata creation
  - File upload with multipart form data
  - Download link creation
  - File download and content verification
  - Admin metrics endpoint
  - File deletion
  - Logout

### Helper Functions

- `setupTestServer()`: Initializes test server with all dependencies
- Placeholder for future test utilities (database cleanup, test data generation, etc.)

## Adding New Tests

When adding integration tests:

1. **Use the integration build tag**:
   ```go
   //go:build integration
   // +build integration
   ```

2. **Follow the AAA pattern**:
   - **Arrange**: Set up test data and dependencies
   - **Act**: Execute the API call
   - **Assert**: Verify the response

3. **Clean up after tests**:
   ```go
   t.Cleanup(func() {
       // Delete test data
       // Close connections
   })
   ```

4. **Use subtests for organization**:
   ```go
   t.Run("Test Case Name", func(t *testing.T) {
       // Test code
   })
   ```

5. **Test both success and failure cases**:
   ```go
   t.Run("Success Case", func(t *testing.T) { ... })
   t.Run("Invalid Input", func(t *testing.T) { ... })
   t.Run("Unauthorized Access", func(t *testing.T) { ... })
   ```

## Best Practices

### Test Isolation

Each test should be independent:
- Don't rely on test execution order
- Clean up test data after each test
- Use unique identifiers (UUIDs, timestamps) for test data

### Realistic Scenarios

Test real-world usage:
- Use realistic file sizes and content
- Test rate limiting and quotas
- Verify concurrent upload handling
- Test expired link behavior

### Error Cases

Don't just test happy paths:
- Invalid authentication
- Exceeded quotas
- Malformed requests
- Missing files
- Network timeouts

### Performance

Monitor test execution time:
```bash
go test -v -tags=integration -bench=. ./tests/integration/
```

## Troubleshooting

### Tests Fail to Connect to Database

```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Check connection
psql -U postgres -h localhost -d sfd_test -c "SELECT 1;"
```

### Tests Fail to Connect to MinIO

```bash
# Check MinIO is running
docker-compose ps minio

# Test MinIO connection
mc alias set local http://localhost:9000 minioadmin minioadmin
mc ls local
```

### Tests Timeout

Increase timeout:
```bash
go test -v -tags=integration -timeout 5m ./tests/integration/
```

### Session Cookie Not Set

Verify session secret is configured:
```bash
echo $SFD_SESSION_SECRET
# Should be at least 32 characters
```

## Future Enhancements

Planned improvements:

- [ ] Concurrent upload tests
- [ ] Large file upload tests (>100MB)
- [ ] Rate limiting verification
- [ ] Link expiration tests
- [ ] Admin user management tests
- [ ] Database migration tests
- [ ] S3-compatible storage tests (AWS S3, Backblaze B2)
- [ ] Performance benchmarks
- [ ] Chaos testing (service failures)

## Related Documentation

- [API Reference](../../docs/API.md) - Complete API endpoint documentation
- [Contributing Guide](../../docs/CONTRIBUTING.md) - How to contribute tests
- [Deployment Guide](../../docs/DEPLOYMENT.md) - Production deployment setup
- [Main README](../../README.md) - Project overview

## Support

For questions about integration tests:
1. Check existing test examples in `api_test.go`
2. Review the [Contributing Guide](../../docs/CONTRIBUTING.md)
3. Open an issue with the `testing` label
