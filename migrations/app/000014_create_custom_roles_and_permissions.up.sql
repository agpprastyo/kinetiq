-- Migration 000014: Custom Roles and Granular CRUD Permissions for Tenant & Platform

-- 1. Tenant Custom Roles
CREATE TABLE IF NOT EXISTS organization_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id TEXT NOT NULL REFERENCES "organization"("id") ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_org_role_name UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS organization_role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES organization_roles(id) ON DELETE CASCADE,
    resource VARCHAR(50) NOT NULL, -- e.g. 'analytics', 'projects', 'api_keys', 'webhooks', 'alerts', 'members', 'billing'
    action VARCHAR(50) NOT NULL,   -- e.g. 'create', 'read', 'update', 'delete', 'export'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_org_role_permission UNIQUE (role_id, resource, action)
);

-- Link member table to custom role (optional role_id while maintaining backward-compatible role text)
ALTER TABLE "member"
    ADD COLUMN IF NOT EXISTS "customRoleId" UUID NULL REFERENCES organization_roles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_org_roles_org_id ON organization_roles(organization_id);
CREATE INDEX IF NOT EXISTS idx_org_role_perms_role_id ON organization_role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_member_custom_role ON "member"("customRoleId");

-- 2. Platform Staff Custom Roles
CREATE TABLE IF NOT EXISTS platform_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS platform_role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES platform_roles(id) ON DELETE CASCADE,
    resource VARCHAR(50) NOT NULL, -- e.g. 'telemetry', 'tenants', 'projects', 'dlq', 'financials', 'audit_logs', 'staff'
    action VARCHAR(50) NOT NULL,   -- e.g. 'create', 'read', 'update', 'delete', 'reconcile', 'replay', 'purge'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_platform_role_permission UNIQUE (role_id, resource, action)
);

-- Link user table to platform role (optional platform_role_id while maintaining staffRole text)
ALTER TABLE "user"
    ADD COLUMN IF NOT EXISTS "platformRoleId" UUID NULL REFERENCES platform_roles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_platform_role_perms_role_id ON platform_role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_user_platform_role ON "user"("platformRoleId");

-- 3. Seed Default System Roles for Platform Staff
INSERT INTO platform_roles (id, name, description, is_system) VALUES
    ('00000000-0000-0000-0000-000000000001', 'superadmin', 'Full platform administrative access', TRUE),
    ('00000000-0000-0000-0000-000000000002', 'ops', 'Site reliability and infrastructure operations', TRUE),
    ('00000000-0000-0000-0000-000000000003', 'finance', 'Financial reports and billing reconciliation', TRUE),
    ('00000000-0000-0000-0000-000000000004', 'support', 'Read-only support staff access', TRUE)
ON CONFLICT (name) DO NOTHING;

-- Seed Superadmin Permissions (Full CRUD across all resources)
INSERT INTO platform_role_permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000001', 'telemetry', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'tenants', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'tenants', 'update'),
    ('00000000-0000-0000-0000-000000000001', 'projects', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'projects', 'update'),
    ('00000000-0000-0000-0000-000000000001', 'dlq', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'dlq', 'replay'),
    ('00000000-0000-0000-0000-000000000001', 'dlq', 'purge'),
    ('00000000-0000-0000-0000-000000000001', 'financials', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'financials', 'reconcile'),
    ('00000000-0000-0000-0000-000000000001', 'audit_logs', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'staff', 'create'),
    ('00000000-0000-0000-0000-000000000001', 'staff', 'read'),
    ('00000000-0000-0000-0000-000000000001', 'staff', 'update'),
    ('00000000-0000-0000-0000-000000000001', 'staff', 'delete')
ON CONFLICT DO NOTHING;

-- Seed Ops Permissions
INSERT INTO platform_role_permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000002', 'telemetry', 'read'),
    ('00000000-0000-0000-0000-000000000002', 'tenants', 'read'),
    ('00000000-0000-0000-0000-000000000002', 'projects', 'read'),
    ('00000000-0000-0000-0000-000000000002', 'projects', 'update'),
    ('00000000-0000-0000-0000-000000000002', 'dlq', 'read'),
    ('00000000-0000-0000-0000-000000000002', 'dlq', 'replay'),
    ('00000000-0000-0000-0000-000000000002', 'dlq', 'purge'),
    ('00000000-0000-0000-0000-000000000002', 'audit_logs', 'read')
ON CONFLICT DO NOTHING;

-- Seed Finance Permissions
INSERT INTO platform_role_permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000003', 'financials', 'read'),
    ('00000000-0000-0000-0000-000000000003', 'financials', 'reconcile'),
    ('00000000-0000-0000-0000-000000000003', 'audit_logs', 'read')
ON CONFLICT DO NOTHING;

-- Seed Support Permissions
INSERT INTO platform_role_permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000004', 'telemetry', 'read'),
    ('00000000-0000-0000-0000-000000000004', 'tenants', 'read'),
    ('00000000-0000-0000-0000-000000000004', 'projects', 'read')
ON CONFLICT DO NOTHING;
