# Changelog

All notable changes to Secure File Drop are documented in this file.

## [2.1.0] - 2025-12-30

### 🚀 Production Enhancements - 15 New Features

#### Security Enhancements
- ✅ **Account Lockout**: Automatic lockout after 5 failed login attempts (15min lock, 10min window)
- ✅ **Per-Endpoint Rate Limiting**: Specialized limits for auth (10/min), upload (20/hr), download (100/hr), admin (50/min), API (300/min)
- ✅ **Security Event Notifications**: Email alerts for failed logins, account lockouts, and password changes
- ✅ **Comprehensive Audit Logging**: Database-backed audit trail with JSONB metadata and correlation IDs
- ✅ **Configuration Validation**: Startup validation with detailed error messages for all environment variables

#### Reliability & Performance
- ✅ **Circuit Breaker Pattern**: Fail-fast protection for database (5 failures/30s), MinIO (5/30s), SMTP (3/60s)
- ✅ **Automated Database Backups**: Scheduled PostgreSQL backups with gzip compression, retention policies, and S3 upload
- ✅ **Optimized Connection Pooling**: Tuned settings (25 max, 5 idle, 5min lifetime, 2min idle timeout)
- ✅ **HTTP Compression**: Automatic gzip compression for JSON/HTML responses
- ✅ **Graceful Shutdown**: Clean shutdown with connection draining

#### Observability
- ✅ **Structured JSON Logging**: Production-ready logging with log levels (debug, info, warn, error)
- ✅ **Request Tracing**: Correlation IDs via X-Correlation-ID header for distributed tracing
- ✅ **Prometheus Metrics**: Detailed metrics at `/metrics/prometheus` (requests, uploads, downloads, storage, auth, uptime)
- ✅ **Enhanced Health Checks**: Component-level status for PostgreSQL and MinIO with latency metrics

#### Upload Improvements
- ✅ **Resumable Uploads**: TUS protocol support at `/upload/resumable` for chunked uploads with progress tracking

### Added - Database
- Migration 000010: `resumable_uploads` table for TUS protocol support
- Migration 000011: `audit_logs` table with indexes for security event tracking

### Added - Monitoring
- Prometheus exporter with 7 metric types (requests_total, uploads_total, downloads_total, storage_bytes, login metrics, uptime)
- Request duration percentiles (p50, p95, p99) in Prometheus metrics
- Circuit breaker statistics endpoint

### Added - Configuration
- 15+ new environment variables for backups, logging, monitoring (all optional with sensible defaults)
- Configuration validation at startup with detailed error messages
- Warnings for recommended but missing optional configuration

### Changed
- Middleware stack order optimized: Compression → Security Headers → Rate Limiting → Logging → Tracing
- Database connection pool settings optimized for production workloads
- Rate limiting now per-endpoint instead of global

### Documentation
- New [docs/PRODUCTION_ENHANCEMENTS.md](docs/PRODUCTION_ENHANCEMENTS.md) with complete feature guide
- Updated [README.md](README.md) with production enhancement highlights
- Updated [docs/FEATURES.md](docs/FEATURES.md) with all 15 new capabilities
- Updated [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) with production best practices
- Updated [.env.example](.env.example) with all new environment variables
- Updated [docker-compose.yml](docker-compose.yml) with optional production settings

## [2.0.0] - 2025-12-28

### 🎉 Major Feature Release - 20 Enhancements

#### Phase 1: Core User Features
- ✅ **File Expiration & Auto-Delete**: Configure TTL on uploads with automatic cleanup
- ✅ **Upload History**: View all files uploaded by current user with full metadata
- ✅ **Password-Protected Downloads**: Optional password requirement for download links
- ✅ **Email Verification**: Secure user registration with email confirmation flow
- ✅ **Password Reset**: Self-service password reset via secure email tokens

#### Phase 2: Enhanced UX
- ✅ **Enhanced Upload Progress**: Real-time progress bar with percentage, speed, and ETA
- ✅ **Drag & Drop Upload**: Intuitive file selection with visual feedback
- ✅ **File Type Icons**: Visual icons for PDFs, images, videos, documents, archives
- ✅ **Download QR Codes**: Generate QR codes for easy mobile device sharing

#### Phase 3: Technical Improvements
- ✅ **Rate Limiting**: Token bucket algorithm (100 req/min per IP)
- ✅ **Enhanced Logging**: Request ID, timing, size, client IP, user agent tracking
- ✅ **Health Checks**: `/health` liveness and `/ready` readiness endpoints
- ✅ **Storage Monitoring**: Real-time disk usage metrics in admin dashboard
- ✅ **File Size Limits**: Configurable via `SFD_MAX_UPLOAD_BYTES` with frontend validation

#### Phase 4: Advanced Features
- ✅ **Multi-File Upload**: Queue-based sequential processing of multiple files
- ✅ **Download Statistics**: Track download count and last download timestamp
- ✅ **File Search & Filtering**: Client-side search by name and status filtering
- ✅ **User Storage Quotas**: Per-user limits (10GB default) with enforcement
- ✅ **Email Notifications**: SMTP alerts for uploads, downloads, and deletions
- ✅ **API Documentation**: Comprehensive docs with 25+ endpoints and examples

### Added - UI/UX
- Logout button in header (visible when authenticated)
- Real-time quota usage display with color-coded indicators (75%, 90% thresholds)
- Search bar and status dropdown in admin file listing
- Download count column in file tables
- QR code modal with download option
- Improved error messages and user feedback

### Added - Backend
- 9 database migrations (000001-000009)
- `/quota` endpoint for user storage information
- Email service with HTML templates (Gmail, SendGrid, AWS SES support)
- Download tracking with async updates
- Quota validation before file creation
- File deletion notifications to owners

### Changed
- Enhanced file metadata with `download_count` and `last_downloaded_at`
- Added `storage_quota_bytes` to users table
- Improved session management with logout support
- Better error handling throughout API
- Modernized frontend with responsive design

### Fixed
- JavaScript syntax errors preventing UI interactions
- Duplicate event listener initialization
- Password validation edge cases
- Email uniqueness validation
- Browser cache issues with hard refresh support

### Security
- HMAC-SHA256 signed download tokens
- Bcrypt password hashing (cost 10)
- HttpOnly session cookies
- Rate limiting to prevent abuse
- Parameterized SQL queries (injection prevention)
- Input sanitization (XSS prevention)
- Storage quota enforcement

### Performance
- Asynchronous email sending (non-blocking)
- Async download statistics updates
- Client-side file filtering (instant results)
- Optimized database queries with indexes
- Efficient token bucket rate limiting

### Documentation
- **NEW**: [docs/API.md](docs/API.md) - Complete API reference
- **NEW**: [docs/EMAIL_NOTIFICATIONS.md](docs/EMAIL_NOTIFICATIONS.md) - SMTP setup guide
- Updated README with all 20 features
- Added JavaScript and cURL examples
- Documented all environment variables
- Version history and upgrade guide

### Environment Variables (New)
```bash
# Email Notifications
SFD_EMAIL_ENABLED=true|false
SFD_SMTP_HOST=smtp.example.com
SFD_SMTP_PORT=587
SFD_SMTP_USER=user@example.com
SFD_SMTP_PASSWORD=app-password
SFD_FROM_EMAIL=noreply@example.com
SFD_BASE_URL=https://yourdomain.com

# File Size Limits
SFD_MAX_UPLOAD_BYTES=53687091200  # 50GB default

# Cleanup (existing, documented)
SFD_CLEANUP_ENABLED=true
SFD_CLEANUP_INTERVAL=1h
SFD_CLEANUP_MAX_AGE=24h
```

## [1.0.0] - 2025-12-26

### Initial MVP Release
- User registration and authentication
- Secure file uploads to MinIO
- SHA-256 integrity verification
- Signed, time-limited download links
- Admin dashboard
- PostgreSQL with migrations
- Docker Compose deployment
- Basic web UI
- Automated cleanup
- Native C hashing utility

---

## Upgrade Guide

### v1.0.0 → v2.0.0

**Prerequisites:**
- Docker Compose installed
- Backup your database before upgrading

**Steps:**
1. Pull latest code: `git pull origin feature/v2-enhancements`
2. Rebuild containers: `docker compose build backend`
3. Restart services: `docker compose up -d`
4. Migrations auto-apply on startup (000001-000009)
5. Configure optional email settings (see above)

**Breaking Changes:** None - fully backward compatible

**New Capabilities:**
- Multi-file uploads
- Email notifications
- Storage quotas
- Download tracking
- File search
- QR codes
- Enhanced UI

**Testing:**
```bash
# Verify health
curl http://localhost:8080/health

# Check version
curl http://localhost:8080/version

# Test login
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' \
  -c cookies.txt
```

## Version History
- **v2.0.0** (2025-12-28): 20 major enhancements
- **v1.0.0** (2025-12-26): MVP release
