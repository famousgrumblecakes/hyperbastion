import { SQL } from "bun";

// Single SQLite connection for the manager service.
// TODO: path should come from an environment variable or config file.
export const db = new SQL("sqlite://app.db", {
    create: true, // Create database if it doesn't exist
});
// ---------------------------------------------------------------------------
// Schema
// Each statement is awaited individually — SQLite doesn't support the
// multi-statement "simple" mode that PostgreSQL does.
// ---------------------------------------------------------------------------

await db`
    CREATE TABLE IF NOT EXISTS users (
        id                  INTEGER  PRIMARY KEY AUTOINCREMENT,
        username            TEXT     NOT NULL UNIQUE,
        email               TEXT     NOT NULL UNIQUE,

        -- Human-readable name sourced from the SSO provider's profile claim
        -- (e.g. "displayName" in Microsoft Graph) or set manually for local users.
        display_name        TEXT,

        -- Tracks how this user authenticates. 'local' means a password stored in
        -- this table. Future values: 'microsoft', 'google', etc.
        -- Together with provider_user_id this forms a unique identity per provider.
        auth_provider       TEXT     NOT NULL DEFAULT 'local',

        -- The unique identifier issued by the SSO provider (e.g. Microsoft Entra
        -- Object ID from the 'oid' claim). NULL for local users.
        -- SQLite treats each NULL as distinct, so the UNIQUE constraint below
        -- correctly allows many local users while enforcing uniqueness per SSO user.
        provider_user_id    TEXT,

        -- Bcrypt hash. NULL for SSO users who never set a local password.
        password_hash       TEXT,

        is_active           INTEGER  NOT NULL DEFAULT 1,
        created_at          TEXT     NOT NULL DEFAULT (datetime('now')),
        updated_at          TEXT     NOT NULL DEFAULT (datetime('now')),
        last_login_at       TEXT,

        UNIQUE (auth_provider, provider_user_id)
    )
`;

// Keep updated_at current automatically on any UPDATE.
await db`
    CREATE TRIGGER IF NOT EXISTS users_set_updated_at
    AFTER UPDATE ON users
    BEGIN
        UPDATE users SET updated_at = datetime('now') WHERE id = NEW.id;
    END
`;
