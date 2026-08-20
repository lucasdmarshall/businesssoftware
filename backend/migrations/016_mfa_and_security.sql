-- Privileged-user MFA foundation and security metadata.

-- TOTP enrollment. A row exists once a user starts enrollment; enabled flips to
-- true only after they confirm a valid code.
CREATE TABLE IF NOT EXISTS user_mfa (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Track when a user was offboarded for audit and access-revocation reporting.
ALTER TABLE users ADD COLUMN IF NOT EXISTS offboarded_at TIMESTAMPTZ;
