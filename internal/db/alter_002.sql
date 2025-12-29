-- Migration: Add audit logging and webhook tables
-- Version: 002
-- Date: 2025-01-29

-- Audit logs table
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    action VARCHAR(50) NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    username VARCHAR(255),
    ip_address VARCHAR(45) NOT NULL,
    user_agent TEXT,
    resource VARCHAR(255),
    details JSONB,
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for audit logs
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource);

-- Webhook configurations table
CREATE TABLE IF NOT EXISTS webhook_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url TEXT NOT NULL,
    events TEXT[] NOT NULL,
    secret VARCHAR(255),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    retry_count INTEGER NOT NULL DEFAULT 3,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Webhook delivery logs table
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_config_id UUID REFERENCES webhook_configs(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    event VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    success BOOLEAN NOT NULL,
    error_message TEXT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    response_status INTEGER,
    response_body TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0
);

-- Indexes for webhook deliveries
CREATE INDEX idx_webhook_deliveries_timestamp ON webhook_deliveries(timestamp DESC);
CREATE INDEX idx_webhook_deliveries_event ON webhook_deliveries(event);
CREATE INDEX idx_webhook_deliveries_success ON webhook_deliveries(success);
CREATE INDEX idx_webhook_deliveries_config_id ON webhook_deliveries(webhook_config_id);

-- Add comments
COMMENT ON TABLE audit_logs IS 'Comprehensive audit trail for all system actions';
COMMENT ON TABLE webhook_configs IS 'Webhook endpoint configurations';
COMMENT ON TABLE webhook_deliveries IS 'Log of all webhook delivery attempts';

-- Grant permissions (adjust as needed for your setup)
-- GRANT SELECT, INSERT ON audit_logs TO sfd_app_user;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON webhook_configs TO sfd_app_user;
-- GRANT SELECT, INSERT ON webhook_deliveries TO sfd_app_user;
