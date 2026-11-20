-- Migration 000014 Down: Drop Custom Roles and Permissions

ALTER TABLE "user" DROP COLUMN IF EXISTS "platformRoleId";
DROP TABLE IF EXISTS platform_role_permissions;
DROP TABLE IF EXISTS platform_roles;

ALTER TABLE "member" DROP COLUMN IF EXISTS "customRoleId";
DROP TABLE IF EXISTS organization_role_permissions;
DROP TABLE IF EXISTS organization_roles;
