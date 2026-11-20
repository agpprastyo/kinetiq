-- Migration 000013: Add Platform Staff Roles to Better-Auth User Table
ALTER TABLE "user"
    ADD COLUMN IF NOT EXISTS "isPlatformStaff" BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS "staffRole" VARCHAR(50) DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_user_platform_staff ON "user" ("isPlatformStaff") WHERE "isPlatformStaff" = TRUE;
