-- Rollback link password feature
ALTER TABLE files DROP COLUMN IF EXISTS link_password;
