DROP INDEX IF EXISTS idx_user_platform_staff;
ALTER TABLE "user"
    DROP COLUMN IF EXISTS "isPlatformStaff",
    DROP COLUMN IF EXISTS "staffRole";
