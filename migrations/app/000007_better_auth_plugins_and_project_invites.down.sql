DROP INDEX IF EXISTS idx_invitation_email;
DROP INDEX IF EXISTS idx_invitation_org_id;
DROP INDEX IF EXISTS idx_member_user_id;
DROP INDEX IF EXISTS idx_member_org_id;

DROP TABLE IF EXISTS "invitation";
DROP TABLE IF EXISTS "member";
DROP TABLE IF EXISTS "organization";
DROP TABLE IF EXISTS "twoFactor";

ALTER TABLE "session"
    DROP COLUMN IF EXISTS "impersonatedBy",
    DROP COLUMN IF EXISTS "activeOrganizationId";

ALTER TABLE "user"
    DROP COLUMN IF EXISTS "banExpires",
    DROP COLUMN IF EXISTS "banReason",
    DROP COLUMN IF EXISTS "banned",
    DROP COLUMN IF EXISTS "role",
    DROP COLUMN IF EXISTS "twoFactorEnabled";
