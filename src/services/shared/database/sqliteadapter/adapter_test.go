//go:build desktop

package sqliteadapter

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"arkloop/services/shared/database"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *Pool {
	t.Helper()
	pool, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func migratedTestDB(t *testing.T) *Pool {
	t.Helper()
	pool, err := AutoMigrate(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("auto-migrate test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func createTestTable(t *testing.T, pool *Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`CREATE TABLE test_items (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	if err != nil {
		t.Fatalf("create test table: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pool / Open
// ---------------------------------------------------------------------------

func TestOpen(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestOpen_Pragmas(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()

	// In-memory databases cannot use WAL; they report "memory".
	var journalMode string
	if err := pool.QueryRow(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "memory" {
		t.Errorf("journal_mode = %q; want %q", journalMode, "memory")
	}

	var fk int
	if err := pool.QueryRow(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d; want 1", fk)
	}
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

func TestAutoMigrate(t *testing.T) {
	t.Parallel()
	pool := migratedTestDB(t)
	ctx := context.Background()

	// Verify that at least one application table exists after the orgs -> accounts rename.
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='accounts'`).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("accounts table not found after auto-migrate")
	}

	// Verify _sequences table exists (needed by SQLiteDialect.Sequence()).
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM _sequences WHERE name = 'run_events_seq_global'`).Scan(&count)
	if err != nil {
		t.Fatalf("query _sequences: %v", err)
	}
	if count != 1 {
		t.Fatalf("run_events_seq_global row not found in _sequences after auto-migrate")
	}

	for _, tableName := range []string{
		"channels",
		"channel_identities",
		"channel_identity_bind_codes",
		"channel_dm_threads",
		"channel_message_receipts",
		"channel_message_ledger",
	} {
		err = pool.QueryRow(ctx,
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`,
			tableName,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query sqlite_master for %s: %v", tableName, err)
		}
		if count != 1 {
			t.Fatalf("%s table not found after auto-migrate", tableName)
		}
	}

	columns, err := sqliteTableColumns(ctx, pool.Unwrap(), "platform_settings")
	if err != nil {
		t.Fatalf("load platform_settings columns: %v", err)
	}
	if !hasSQLiteColumns(columns, "key", "value", "updated_at") {
		t.Fatalf("platform_settings columns = %v, want key/value/updated_at", columns)
	}
}

func TestAutoMigrateRepairsLegacySecretsSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "desktop.db")

	pool, err := AutoMigrate(ctx, dbPath)
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, status)
		VALUES (?, 'desktop', 'desktop@localhost', 'active')`,
		desktopCompatUserID,
	); err != nil {
		t.Fatalf("seed desktop user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, slug, name, type, owner_user_id)
		VALUES (?, 'desktop', 'Desktop', 'personal', ?)`,
		desktopCompatAccountID, desktopCompatUserID,
	); err != nil {
		t.Fatalf("seed desktop account: %v", err)
	}

	for _, stmt := range []string{
		`PRAGMA foreign_keys = OFF`,
		`DROP INDEX IF EXISTS secrets_platform_name_idx`,
		`DROP INDEX IF EXISTS secrets_user_name_idx`,
		`ALTER TABLE secrets RENAME TO secrets_aligned_backup`,
		`CREATE TABLE secrets (
			id              TEXT PRIMARY KEY,
			account_id      TEXT NOT NULL,
			name            TEXT NOT NULL,
			encrypted_value TEXT NOT NULL,
			key_version     INTEGER NOT NULL DEFAULT 1,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(account_id, name)
		)`,
		`DROP TABLE channels`,
		`CREATE TABLE channels (
			id             TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			account_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			channel_type   TEXT NOT NULL,
			persona_id     TEXT REFERENCES personas(id) ON DELETE SET NULL,
			credentials_id TEXT REFERENCES secrets(id),
			webhook_secret TEXT,
			webhook_url    TEXT,
			is_active      INTEGER NOT NULL DEFAULT 0,
			config_json    TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (account_id, channel_type)
		)`,
		`DROP TABLE secrets_aligned_backup`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("downgrade secrets schema: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO secrets (id, account_id, name, encrypted_value, key_version)
		VALUES (?, ?, 'legacy-bot-token', 'ciphertext', 7)`,
		"11111111-1111-4111-8111-111111111111",
		desktopCompatAccountID,
	); err != nil {
		t.Fatalf("insert legacy secret: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("close sqlite before reopen: %v", err)
	}

	repairedPool, err := AutoMigrate(ctx, dbPath)
	if err != nil {
		t.Fatalf("repair auto migrate sqlite: %v", err)
	}
	defer repairedPool.Close()

	columns, err := sqliteTableColumns(ctx, repairedPool.Unwrap(), "secrets")
	if err != nil {
		t.Fatalf("load repaired secrets columns: %v", err)
	}
	if !hasSQLiteColumns(columns, "owner_kind", "owner_user_id", "rotated_at") {
		t.Fatalf("repaired secrets table missing owner columns: %v", columns)
	}

	var (
		ownerKind   string
		ownerUserID sql.NullString
		name        string
		keyVersion  int
		rotatedAt   sql.NullString
	)
	if err := repairedPool.QueryRow(ctx, `
		SELECT owner_kind, owner_user_id, name, key_version, rotated_at
		FROM secrets
		WHERE id = ?`,
		"11111111-1111-4111-8111-111111111111",
	).Scan(&ownerKind, &ownerUserID, &name, &keyVersion, &rotatedAt); err != nil {
		t.Fatalf("query repaired secret: %v", err)
	}
	if ownerKind != "user" {
		t.Fatalf("owner_kind = %q, want user", ownerKind)
	}
	if !ownerUserID.Valid || ownerUserID.String != desktopCompatUserID {
		t.Fatalf("owner_user_id = %#v, want %s", ownerUserID, desktopCompatUserID)
	}
	if name != "legacy-bot-token" {
		t.Fatalf("name = %q, want legacy-bot-token", name)
	}
	if keyVersion != 7 {
		t.Fatalf("key_version = %d, want 7", keyVersion)
	}
	if rotatedAt.Valid {
		t.Fatalf("rotated_at = %#v, want NULL", rotatedAt)
	}

	channelColumns, err := sqliteTableColumns(ctx, repairedPool.Unwrap(), "channels")
	if err != nil {
		t.Fatalf("load repaired channels columns: %v", err)
	}
	if !hasSQLiteColumns(channelColumns, "owner_user_id") {
		t.Fatalf("repaired channels table missing owner_user_id: %v", channelColumns)
	}
}

func TestAutoMigrateRepairsLegacyChannelOwnerColumn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "desktop.db")

	pool, err := AutoMigrate(ctx, dbPath)
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}

	for _, stmt := range []string{
		`PRAGMA foreign_keys = OFF`,
		`DROP INDEX IF EXISTS idx_channels_account_id`,
		`DROP TABLE channels`,
		`CREATE TABLE channels (
			id             TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			account_id     TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			channel_type   TEXT NOT NULL,
			persona_id     TEXT REFERENCES personas(id) ON DELETE SET NULL,
			credentials_id TEXT REFERENCES secrets(id),
			webhook_secret TEXT,
			webhook_url    TEXT,
			is_active      INTEGER NOT NULL DEFAULT 0,
			config_json    TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (account_id, channel_type)
		)`,
		`CREATE INDEX idx_channels_account_id ON channels(account_id)`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare legacy channels schema: %v", err)
		}
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("close sqlite before reopen: %v", err)
	}

	repairedPool, err := AutoMigrate(ctx, dbPath)
	if err != nil {
		t.Fatalf("repair auto migrate sqlite: %v", err)
	}
	defer repairedPool.Close()

	channelColumns, err := sqliteTableColumns(ctx, repairedPool.Unwrap(), "channels")
	if err != nil {
		t.Fatalf("load repaired channels columns: %v", err)
	}
	if !hasSQLiteColumns(channelColumns, "owner_user_id") {
		t.Fatalf("repaired channels table missing owner_user_id: %v", channelColumns)
	}
}

func TestAutoMigrateUpgradesChannelHeartbeatScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "desktop.db")

	pool, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	for _, stmt := range []string{
		`CREATE TABLE goose_db_version (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version_id BIGINT NOT NULL,
			is_applied BOOLEAN NOT NULL,
			tstamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE channel_identities (
			id TEXT PRIMARY KEY,
			channel_type TEXT NOT NULL,
			platform_subject_id TEXT NOT NULL,
			heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
			heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30,
			heartbeat_model TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE channels (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL
		)`,
		`CREATE TABLE personas (
			id TEXT PRIMARY KEY,
			account_id TEXT,
			persona_key TEXT
		)`,
		`CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			deleted_at TEXT
		)`,
		`CREATE TABLE channel_dm_threads (
			channel_id TEXT NOT NULL,
			channel_identity_id TEXT NOT NULL,
			thread_id TEXT NOT NULL
		)`,
		`CREATE TABLE channel_group_threads (
			channel_id TEXT NOT NULL,
			platform_chat_id TEXT NOT NULL,
			persona_id TEXT,
			thread_id TEXT NOT NULL
		)`,
		`CREATE TABLE channel_identity_links (
			id TEXT PRIMARY KEY,
			channel_id TEXT NOT NULL,
			channel_identity_id TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (channel_id, channel_identity_id)
		)`,
		`CREATE TABLE scheduled_triggers (
			id TEXT PRIMARY KEY,
			channel_identity_id TEXT NOT NULL UNIQUE,
			persona_key TEXT NOT NULL,
			account_id TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			interval_min INTEGER NOT NULL DEFAULT 30,
			next_fire_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		// Stubs so migrations 00048+ (MCP installs) can run on this minimal legacy DB.
		`CREATE TABLE users (id TEXT PRIMARY KEY)`,
		`CREATE TABLE accounts (id TEXT PRIMARY KEY)`,
		`CREATE TABLE secrets (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			owner_kind TEXT NOT NULL DEFAULT 'platform',
			owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			name TEXT NOT NULL,
			encrypted_value TEXT NOT NULL,
			key_version INTEGER NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			rotated_at TEXT
		)`,
		`CREATE TABLE workspace_registries (
			workspace_ref TEXT PRIMARY KEY,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE profile_registries (
			profile_ref TEXT PRIMARY KEY,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			default_workspace_ref TEXT,
			owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE account_memberships (
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (account_id, user_id)
		)`,
		`CREATE TABLE mcp_configs (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			name TEXT NOT NULL DEFAULT '',
			transport TEXT NOT NULL DEFAULT 'stdio',
			url TEXT,
			command TEXT,
			args_json TEXT,
			env_json TEXT,
			cwd TEXT,
			call_timeout_ms INTEGER,
			auth_secret_id TEXT REFERENCES secrets(id) ON DELETE SET NULL
		)`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("prepare legacy sqlite schema: %v", err)
		}
	}
	for v := int64(0); v <= 44; v++ {
		if _, err := pool.Exec(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)`, v); err != nil {
			t.Fatalf("seed sqlite goose version %d: %v", v, err)
		}
	}

	accountID := uuid.NewString()
	dmChannelID := uuid.NewString()
	groupChannelID := uuid.NewString()
	dmIdentityID := uuid.NewString()
	groupIdentityID := uuid.NewString()
	dmThreadID := uuid.NewString()
	groupThreadID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO channels (id, account_id) VALUES (?, ?), (?, ?)`, []any{dmChannelID, accountID, groupChannelID, accountID}},
		{`INSERT INTO threads (id, account_id, deleted_at) VALUES (?, ?, NULL), (?, ?, NULL)`, []any{dmThreadID, accountID, groupThreadID, accountID}},
		{`INSERT INTO channel_identities (id, channel_type, platform_subject_id, heartbeat_enabled, heartbeat_interval_minutes, heartbeat_model)
		   VALUES (?, 'discord', 'user-42', 1, 17, 'discord-model'),
		          (?, 'telegram', 'chat-1001', 1, 9, 'group-model')`, []any{dmIdentityID, groupIdentityID}},
		{`INSERT INTO channel_identity_links (id, channel_id, channel_identity_id) VALUES (?, ?, ?)`, []any{uuid.NewString(), dmChannelID, dmIdentityID}},
		{`INSERT INTO channel_dm_threads (channel_id, channel_identity_id, thread_id) VALUES (?, ?, ?)`, []any{dmChannelID, dmIdentityID, dmThreadID}},
		{`INSERT INTO channel_group_threads (channel_id, platform_chat_id, persona_id, thread_id) VALUES (?, 'chat-1001', NULL, ?)`, []any{groupChannelID, groupThreadID}},
		{`INSERT INTO scheduled_triggers (id, channel_identity_id, persona_key, account_id, model, interval_min, next_fire_at, created_at, updated_at)
		   VALUES (?, ?, 'discord-persona', ?, 'discord-model', 17, ?, ?, ?),
		          (?, ?, 'group-persona', ?, 'group-model', 9, ?, ?, ?)`,
			[]any{uuid.NewString(), dmIdentityID, accountID, now, now, now, uuid.NewString(), groupIdentityID, accountID, now, now, now}},
	} {
		if _, err := pool.Exec(ctx, stmt.query, stmt.args...); err != nil {
			t.Fatalf("seed legacy sqlite data: %v", err)
		}
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("close sqlite before reopen: %v", err)
	}

	upgradedPool, err := AutoMigrate(ctx, dbPath)
	if err != nil {
		t.Fatalf("upgrade sqlite auto migrate: %v", err)
	}
	defer upgradedPool.Close()

	linkColumns, err := sqliteTableColumns(ctx, upgradedPool.Unwrap(), "channel_identity_links")
	if err != nil {
		t.Fatalf("load upgraded link columns: %v", err)
	}
	if !hasSQLiteColumns(linkColumns, "heartbeat_enabled", "heartbeat_interval_minutes", "heartbeat_model") {
		t.Fatalf("upgraded link columns missing heartbeat fields: %v", linkColumns)
	}

	var (
		enabled  int
		interval int
		model    string
	)
	if err := upgradedPool.QueryRow(ctx, `
		SELECT heartbeat_enabled, heartbeat_interval_minutes, heartbeat_model
		  FROM channel_identity_links
		 WHERE channel_id = ? AND channel_identity_id = ?`,
		dmChannelID,
		dmIdentityID,
	).Scan(&enabled, &interval, &model); err != nil {
		t.Fatalf("read upgraded sqlite binding heartbeat config: %v", err)
	}
	if enabled != 1 || interval != 17 || model != "discord-model" {
		t.Fatalf("unexpected upgraded sqlite binding heartbeat config: enabled=%d interval=%d model=%q", enabled, interval, model)
	}

	scheduledColumns, err := sqliteTableColumns(ctx, upgradedPool.Unwrap(), "scheduled_triggers")
	if err != nil {
		t.Fatalf("load upgraded scheduled_triggers columns: %v", err)
	}
	if !hasSQLiteColumns(scheduledColumns, "channel_id", "channel_identity_id") {
		t.Fatalf("upgraded scheduled_triggers missing channel_id: %v", scheduledColumns)
	}

	var migratedDMChannelID string
	if err := upgradedPool.QueryRow(ctx, `
		SELECT channel_id
		  FROM scheduled_triggers
		 WHERE channel_identity_id = ? AND persona_key = 'discord-persona'`,
		dmIdentityID,
	).Scan(&migratedDMChannelID); err != nil {
		t.Fatalf("read upgraded sqlite dm trigger: %v", err)
	}
	if migratedDMChannelID != dmChannelID {
		t.Fatalf("sqlite dm trigger channel_id = %q, want %q", migratedDMChannelID, dmChannelID)
	}
}

func TestMigrations_UpDown(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	db := pool.Unwrap()

	// Up
	results, err := Up(ctx, db)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("up returned zero results")
	}

	ver, err := CurrentVersion(ctx, db)
	if err != nil {
		t.Fatalf("current version: %v", err)
	}
	if ver != ExpectedVersion {
		t.Errorf("version after up = %d; want %d", ver, ExpectedVersion)
	}

	// DownAll
	count, err := DownAll(ctx, db)
	if err != nil {
		t.Fatalf("down all: %v", err)
	}
	if count == 0 {
		t.Fatal("down all rolled back zero migrations")
	}

	ver, err = CurrentVersion(ctx, db)
	if err != nil {
		t.Fatalf("current version after down: %v", err)
	}
	if ver != 0 {
		t.Errorf("version after down all = %d; want 0", ver)
	}
}

// ---------------------------------------------------------------------------
// Exec / Query / QueryRow
// ---------------------------------------------------------------------------

func TestExec(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	res, err := pool.Exec(ctx, `INSERT INTO test_items (id, name) VALUES ('1', 'alpha')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if res.RowsAffected() != 1 {
		t.Errorf("rows affected = %d; want 1", res.RowsAffected())
	}
}

func TestQueryRow(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	_, err := pool.Exec(ctx, `INSERT INTO test_items (id, name) VALUES ('1', 'alpha')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var id, name string
	if err := pool.QueryRow(ctx, `SELECT id, name FROM test_items WHERE id = '1'`).Scan(&id, &name); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if id != "1" || name != "alpha" {
		t.Errorf("got id=%q name=%q; want id=%q name=%q", id, name, "1", "alpha")
	}
}

func TestQueryRow_NoRows(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	var id string
	err := pool.QueryRow(ctx, `SELECT id FROM test_items WHERE id = 'nope'`).Scan(&id)
	if err == nil {
		t.Fatal("expected error for missing row, got nil")
	}
	if !database.IsNoRows(err) {
		t.Errorf("expected database.ErrNoRows; got %v", err)
	}
}

func TestQuery(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		_, err := pool.Exec(ctx, `INSERT INTO test_items (id, name) VALUES (?, ?)`, name, name)
		if err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
	}

	rows, err := pool.Query(ctx, `SELECT id, name FROM test_items ORDER BY name`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows; want 3", len(got))
	}
	if got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Errorf("got %v; want [alpha beta gamma]", got)
	}
}

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

func TestTransaction_Commit(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	txn, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, err = txn.Exec(ctx, `INSERT INTO test_items (id, name) VALUES ('1', 'alpha')`)
	if err != nil {
		t.Fatalf("exec in tx: %v", err)
	}
	if err := txn.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM test_items WHERE id = '1'`).Scan(&name); err != nil {
		t.Fatalf("select after commit: %v", err)
	}
	if name != "alpha" {
		t.Errorf("name = %q; want %q", name, "alpha")
	}
}

func TestTransaction_Rollback(t *testing.T) {
	t.Parallel()
	pool := openTestDB(t)
	ctx := context.Background()
	createTestTable(t, pool)

	txn, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, err = txn.Exec(ctx, `INSERT INTO test_items (id, name) VALUES ('1', 'alpha')`)
	if err != nil {
		t.Fatalf("exec in tx: %v", err)
	}
	if err := txn.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM test_items`).Scan(&count); err != nil {
		t.Fatalf("select after rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d; want 0 after rollback", count)
	}
}

// ---------------------------------------------------------------------------
// Dialect
// ---------------------------------------------------------------------------

func TestDialect_Name(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	if d.Name() != database.DialectSQLite {
		t.Errorf("Name() = %q; want %q", d.Name(), database.DialectSQLite)
	}
}

func TestDialect_Placeholder(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	tests := []struct {
		index int
		want  string
	}{
		{1, "?1"},
		{3, "?3"},
		{10, "?10"},
	}
	for _, tt := range tests {
		if got := d.Placeholder(tt.index); got != tt.want {
			t.Errorf("Placeholder(%d) = %q; want %q", tt.index, got, tt.want)
		}
	}
}

func TestDialect_Now(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	if got := d.Now(); got != "datetime('now')" {
		t.Errorf("Now() = %q; want %q", got, "datetime('now')")
	}
}

func TestDialect_IntervalAdd(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	got := d.IntervalAdd("created_at", "24 hours", "+24 hours")
	want := "datetime(created_at, '+24 hours')"
	if got != want {
		t.Errorf("IntervalAdd() = %q; want %q", got, want)
	}
}

func TestDialect_JSONCast(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	expr := "some_column"
	if got := d.JSONCast(expr); got != expr {
		t.Errorf("JSONCast(%q) = %q; want %q (no-op)", expr, got, expr)
	}
}

func TestDialect_ForUpdate(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	if got := d.ForUpdate(); got != "" {
		t.Errorf("ForUpdate() = %q; want empty string", got)
	}
}

func TestDialect_ILike(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	if got := d.ILike(); got != "LIKE" {
		t.Errorf("ILike() = %q; want %q", got, "LIKE")
	}
}

func TestDialect_ArrayAny(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}
	got := d.ArrayAny("status", 2)
	want := "EXISTS(SELECT 1 FROM json_each(?2) WHERE value = status)"
	if got != want {
		t.Errorf("ArrayAny() = %q; want %q", got, want)
	}
}

func TestDialect_OnConflict(t *testing.T) {
	t.Parallel()
	d := SQLiteDialect{}

	gotUpdate := d.OnConflictDoUpdate("id", "name = excluded.name")
	wantUpdate := "ON CONFLICT (id) DO UPDATE SET name = excluded.name"
	if gotUpdate != wantUpdate {
		t.Errorf("OnConflictDoUpdate() = %q; want %q", gotUpdate, wantUpdate)
	}

	gotNothing := d.OnConflictDoNothing("id")
	wantNothing := "ON CONFLICT (id) DO NOTHING"
	if gotNothing != wantNothing {
		t.Errorf("OnConflictDoNothing() = %q; want %q", gotNothing, wantNothing)
	}
}

// ---------------------------------------------------------------------------
// Embedded migrations metadata
// ---------------------------------------------------------------------------

func TestEmbeddedMigrations(t *testing.T) {
	t.Parallel()
	if ExpectedVersion <= 0 {
		t.Errorf("ExpectedVersion = %d; want > 0", ExpectedVersion)
	}
	if EmbeddedMigrationCount <= 0 {
		t.Errorf("EmbeddedMigrationCount = %d; want > 0", EmbeddedMigrationCount)
	}
}
