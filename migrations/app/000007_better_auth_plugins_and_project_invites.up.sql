-- 1. Extend Better-Auth "user" table for twoFactor and admin plugins
ALTER TABLE "user"
    ADD COLUMN IF NOT EXISTS "twoFactorEnabled" BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS "role" TEXT NOT NULL DEFAULT 'owner',
    ADD COLUMN IF NOT EXISTS "banned" BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS "banReason" TEXT,
    ADD COLUMN IF NOT EXISTS "banExpires" TIMESTAMPTZ;

-- 2. Extend Better-Auth "session" table for organization and admin plugins
ALTER TABLE "session"
    ADD COLUMN IF NOT EXISTS "activeOrganizationId" TEXT,
    ADD COLUMN IF NOT EXISTS "impersonatedBy" TEXT;

-- 3. Create Better-Auth "twoFactor" table
CREATE TABLE IF NOT EXISTS "twoFactor" (
    "id" TEXT PRIMARY KEY,
    "secret" TEXT NOT NULL,
    "backupCodes" TEXT NOT NULL,
    "userId" TEXT NOT NULL REFERENCES "user"("id") ON DELETE CASCADE,
    "verified" BOOLEAN DEFAULT FALSE,
    "failedVerificationCount" INTEGER DEFAULT 0,
    "lockedUntil" TIMESTAMPTZ
);

-- 4. Create Better-Auth "organization" table (Workspace)
CREATE TABLE IF NOT EXISTS "organization" (
    "id" TEXT PRIMARY KEY,
    "name" TEXT NOT NULL,
    "slug" TEXT NOT NULL UNIQUE,
    "logo" TEXT,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    "metadata" TEXT,
    "plan" TEXT NOT NULL DEFAULT 'starter',
    "billingProvider" TEXT NOT NULL DEFAULT 'midtrans',
    "monthlyEventQuota" BIGINT NOT NULL DEFAULT 100000
);

-- 5. Create Better-Auth "member" table (supports both Workspace & Project RBAC)
CREATE TABLE IF NOT EXISTS "member" (
    "id" TEXT PRIMARY KEY,
    "organizationId" TEXT NOT NULL REFERENCES "organization"("id") ON DELETE CASCADE,
    "userId" TEXT NOT NULL REFERENCES "user"("id") ON DELETE CASCADE,
    "role" TEXT NOT NULL DEFAULT 'member',
    "scopeType" TEXT NOT NULL DEFAULT 'workspace' CHECK ("scopeType" IN ('workspace', 'project')),
    "projectId" UUID NULL REFERENCES projects(id) ON DELETE CASCADE,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. Create Better-Auth "invitation" table (supports inviting to Workspace OR specific Project with Role)
CREATE TABLE IF NOT EXISTS "invitation" (
    "id" TEXT PRIMARY KEY,
    "organizationId" TEXT NOT NULL REFERENCES "organization"("id") ON DELETE CASCADE,
    "email" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'member',
    "scopeType" TEXT NOT NULL DEFAULT 'workspace' CHECK ("scopeType" IN ('workspace', 'project')),
    "projectId" UUID NULL REFERENCES projects(id) ON DELETE CASCADE,
    "projectName" TEXT NULL,
    "status" TEXT NOT NULL DEFAULT 'pending',
    "expiresAt" TIMESTAMPTZ NOT NULL,
    "inviterId" TEXT NOT NULL REFERENCES "user"("id") ON DELETE CASCADE,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_member_org_id ON "member" ("organizationId");
CREATE INDEX IF NOT EXISTS idx_member_user_id ON "member" ("userId");
CREATE INDEX IF NOT EXISTS idx_invitation_org_id ON "invitation" ("organizationId");
CREATE INDEX IF NOT EXISTS idx_invitation_email ON "invitation" ("email");
