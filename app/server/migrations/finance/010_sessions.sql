ALTER TABLE finance_users ADD COLUMN disabled_at TIMESTAMPTZ;
-- Expand only: keep the bcrypt column readable by the previous application.
ALTER TABLE finance_users ADD COLUMN password_hash_v2 TEXT;

CREATE TABLE finance_browser_sessions (
    jti TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES finance_users(id) ON DELETE CASCADE,
    token_digest TEXT NOT NULL UNIQUE CHECK (length(token_digest) = 64),
    password_digest TEXT NOT NULL CHECK (length(password_digest) = 64),
    csrf_digest TEXT CHECK (csrf_digest IS NULL OR length(csrf_digest) = 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (expires_at > created_at)
);
CREATE INDEX idx_finance_browser_sessions_user ON finance_browser_sessions(user_id);
CREATE INDEX idx_finance_browser_sessions_expiry ON finance_browser_sessions(expires_at);

-- Host password recovery and owner disable must revoke even if no browser
-- request observes the change. Keep this invariant for N-1 application writes.
CREATE FUNCTION finance_revoke_browser_sessions() RETURNS trigger AS $$
BEGIN
    IF OLD.password_hash IS DISTINCT FROM NEW.password_hash
       OR OLD.password_hash_v2 IS DISTINCT FROM NEW.password_hash_v2
       OR NEW.disabled_at IS NOT NULL THEN
        UPDATE finance_browser_sessions SET revoked_at = now()
        WHERE user_id = NEW.id AND revoked_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER finance_revoke_browser_sessions
    AFTER UPDATE OF password_hash, password_hash_v2, disabled_at ON finance_users
    FOR EACH ROW EXECUTE FUNCTION finance_revoke_browser_sessions();
