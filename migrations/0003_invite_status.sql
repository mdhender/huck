-- 0003_invite_status.sql -- Add soft-revoke + invite role.
ALTER TABLE invites ADD COLUMN revoked_at TEXT;
ALTER TABLE invites ADD COLUMN is_admin   INTEGER NOT NULL DEFAULT 0;

-- "One active invite per email" must now also exclude revoked
-- rows. Drop+create plainly; the index is guaranteed to exist
-- from 0001_init.sql, and a "missing index" failure here is a
-- legitimate signal of schema drift.
DROP INDEX invites_email_active;
CREATE UNIQUE INDEX invites_email_active
    ON invites(email)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
