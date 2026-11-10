// Package db opens the SQLite database and applies the schema.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (and creates if needed) the SQLite database at path and applies
// the schema. The directory is created when missing.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serializes writers anyway; a single connection avoids lock
	// contention and keeps the pure-Go driver predictable.
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

const providerConfigDDL = `CREATE TABLE IF NOT EXISTS provider_configs (
	id              TEXT PRIMARY KEY,
	name            TEXT NOT NULL,
	provider        TEXT NOT NULL,
	merchant_id     TEXT NOT NULL DEFAULT '',
	store_id        TEXT NOT NULL DEFAULT '',
	account_id      TEXT NOT NULL DEFAULT '',
	static_qris     TEXT NOT NULL DEFAULT '',
	session_enc     TEXT NOT NULL DEFAULT '',
	poll_interval_ms INTEGER NOT NULL DEFAULT 3000,
	expiry_ms       INTEGER NOT NULL DEFAULT 300000,
	clock_skew_ms   INTEGER NOT NULL DEFAULT 60000,
	active          INTEGER NOT NULL DEFAULT 1,
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL
);`

const schema = `
CREATE TABLE IF NOT EXISTS admin_users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);

` + providerConfigDDL + `

CREATE TABLE IF NOT EXISTS settings (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS payments (
	id            TEXT PRIMARY KEY,
	config_id     TEXT NOT NULL,
	reference     TEXT NOT NULL DEFAULT '',
	base_amount   INTEGER NOT NULL,
	unique_offset INTEGER NOT NULL,
	unique_amount INTEGER NOT NULL,
	status        TEXT NOT NULL,
	qr_string     TEXT NOT NULL DEFAULT '',
	provider      TEXT NOT NULL DEFAULT '',
	account_id    TEXT NOT NULL DEFAULT '',
	merchant_id   TEXT NOT NULL DEFAULT '',
	transaction_json TEXT NOT NULL DEFAULT '',
	metadata_json TEXT NOT NULL DEFAULT '',
	created_at    INTEGER NOT NULL,
	expires_at    INTEGER NOT NULL,
	paid_at       INTEGER
);

CREATE INDEX IF NOT EXISTS idx_payments_config ON payments(config_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_created ON payments(created_at DESC);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
	id               TEXT PRIMARY KEY,
	config_id        TEXT NOT NULL,
	payment_id       TEXT NOT NULL,
	url              TEXT NOT NULL,
	payload          TEXT NOT NULL,
	signature        TEXT NOT NULL,
	status           TEXT NOT NULL,
	attempts         INTEGER NOT NULL DEFAULT 0,
	last_status_code INTEGER NOT NULL DEFAULT 0,
	last_response    TEXT NOT NULL DEFAULT '',
	next_attempt_at  INTEGER NOT NULL,
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_webhook_due ON webhook_deliveries(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_webhook_payment ON webhook_deliveries(payment_id);
`

func migrate(conn *sql.DB) error {
	legacy, err := hasColumn(conn, "provider_configs", "api_token")
	if err != nil {
		return err
	}

	if _, err := conn.Exec(schema); err != nil {
		return err
	}
	if !legacy {
		return nil
	}
	return migrateLegacyProviderConfigs(conn)
}

func hasColumn(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			found = true
		}
	}
	return found, rows.Err()
}

type legacySettings struct {
	providerID    string
	apiToken      string
	webhookURL    string
	webhookSecret string
	uniqueMin     int64
	uniqueMax     int64
}

func migrateLegacyProviderConfigs(conn *sql.DB) error {
	var ls legacySettings
	err := conn.QueryRow(`SELECT id, api_token, webhook_url, webhook_secret, unique_min, unique_max
		FROM provider_configs ORDER BY created_at ASC LIMIT 1`).Scan(
		&ls.providerID, &ls.apiToken, &ls.webhookURL, &ls.webhookSecret, &ls.uniqueMin, &ls.uniqueMax,
	)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`ALTER TABLE provider_configs RENAME TO provider_configs_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(providerConfigDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO provider_configs (
			id, name, provider, merchant_id, store_id, account_id, static_qris, session_enc,
			poll_interval_ms, expiry_ms, clock_skew_ms, active, created_at, updated_at)
		SELECT id, name, provider, merchant_id, store_id, account_id, static_qris,
			session_enc, poll_interval_ms, expiry_ms, clock_skew_ms, active, created_at, updated_at
		FROM provider_configs_legacy`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE provider_configs_legacy`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Preserve the previous per-provider values as the new global defaults.
	now := nowMillis()
	seed := []struct {
		key   string
		value string
	}{
		{"api_token", ls.apiToken},
		{"webhook_url", ls.webhookURL},
		{"webhook_secret", ls.webhookSecret},
		{"unique_min", fmt.Sprintf("%d", ls.uniqueMin)},
		{"unique_max", fmt.Sprintf("%d", ls.uniqueMax)},
		{"default_provider_id", ls.providerID},
	}
	for _, s := range seed {
		if s.value == "" || s.value == "0" {
			continue
		}
		if _, err := conn.Exec(
			`INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)`,
			s.key, s.value, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func nowMillis() int64 { return time.Now().UnixMilli() }
