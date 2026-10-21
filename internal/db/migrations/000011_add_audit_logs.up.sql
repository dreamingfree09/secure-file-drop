-- Add audit_logs table for comprehensive audit logging
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    ip_address TEXT NOT NULL,
    user_agent TEXT,
    resource_id TEXT,
    resource_type TEXT,
    action TEXT NOT NULL,
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    metadata JSONB,
    correlation_id TEXT
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ip_address ON audit_logs(ip_address);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_id ON audit_logs(resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_correlation_id ON audit_logs(correlation_id);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_time ON audit_logs(user_id, timestamp DESC);

COMMENT ON TABLE audit_logs IS 'Comprehensive audit log for security-sensitive operations';
COMMENT ON COLUMN audit_logs.event_type IS 'Type of audited event (login, file_upload, etc.)';
COMMENT ON COLUMN audit_logs.metadata IS 'Additional context stored as JSON';
COMMENT ON COLUMN audit_logs.correlation_id IS 'Request correlation ID for distributed tracing';
