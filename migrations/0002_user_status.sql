-- 0002_user_status.sql -- Add login tracking + soft-suspend.
ALTER TABLE users ADD COLUMN last_login_at TEXT;
ALTER TABLE users ADD COLUMN suspended_at  TEXT;
