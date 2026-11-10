// Package repo implements persistence for provider configs, payments, admin
// sessions and webhook deliveries on SQLite.
package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hirotomasato/paygateme/core"
	"github.com/hirotomasato/paygateme/internal/model"
)

// Store is the SQLite repository and also implements core.PaymentStore.
type Store struct {
	db *sql.DB
}

// New wraps an opened database handle.
func New(db *sql.DB) *Store { return &Store{db: db} }

// DB exposes the underlying handle for migrations/tests.
func (s *Store) DB() *sql.DB { return s.db }

func toMS(t time.Time) int64 { return t.UnixMilli() }

func fromMS(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- Admin users & sessions ----

// EnsureAdmin creates the admin account when the table is empty.
func (s *Store) EnsureAdmin(username, passwordHash string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	_, err := s.db.Exec(
		`INSERT INTO admin_users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, toMS(time.Now()),
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

// AdminByUsername loads an admin by username.
func (s *Store) AdminByUsername(username string) (*model.AdminUser, error) {
	var u model.AdminUser
	var created int64
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM admin_users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = fromMS(created)
	return &u, nil
}

// AdminByID loads an admin by id.
func (s *Store) AdminByID(id int64) (*model.AdminUser, error) {
	var u model.AdminUser
	var created int64
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, created_at FROM admin_users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = fromMS(created)
	return &u, nil
}

// CreateAdminSession stores a hashed session token.
func (s *Store) CreateAdminSession(tokenHash string, userID int64, expires time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO admin_sessions (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, toMS(expires), toMS(time.Now()),
	)
	return err
}

// AdminSessionValid reports whether a hashed token belongs to a live session,
// returning the user id when valid.
func (s *Store) AdminSessionValid(tokenHash string) (int64, bool, error) {
	var userID int64
	var expires int64
	err := s.db.QueryRow(
		`SELECT user_id, expires_at FROM admin_sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if time.Now().UnixMilli() >= expires {
		_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, tokenHash)
		return 0, false, nil
	}
	return userID, true, nil
}

// DeleteAdminSession removes a session.
func (s *Store) DeleteAdminSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// ---- Provider configs ----

const providerColumns = `id, name, provider, merchant_id, store_id, account_id, static_qris,
	session_enc, poll_interval_ms, expiry_ms, clock_skew_ms, active, created_at, updated_at`

func scanProvider(sc interface{ Scan(...any) error }) (*model.ProviderConfig, error) {
	var p model.ProviderConfig
	var active int
	var created, updated int64
	if err := sc.Scan(
		&p.ID, &p.Name, &p.Provider, &p.MerchantID, &p.StoreID, &p.AccountID, &p.StaticQRIS,
		&p.SessionEnc, &p.PollIntervalMS, &p.ExpiryMS, &p.ClockSkewMS, &active, &created, &updated,
	); err != nil {
		return nil, err
	}
	p.Active = active == 1
	p.CreatedAt = fromMS(created)
	p.UpdatedAt = fromMS(updated)
	return &p, nil
}

// ListProviderConfigs returns all configured provider accounts.
func (s *Store) ListProviderConfigs() ([]model.ProviderConfig, error) {
	rows, err := s.db.Query(`SELECT ` + providerColumns + ` FROM provider_configs ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ProviderConfig
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetProviderConfig loads one config or returns nil.
func (s *Store) GetProviderConfig(id string) (*model.ProviderConfig, error) {
	row := s.db.QueryRow(`SELECT `+providerColumns+` FROM provider_configs WHERE id = ?`, id)
	p, err := scanProvider(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// CreateProviderConfig inserts a new config.
func (s *Store) CreateProviderConfig(p model.ProviderConfig) error {
	_, err := s.db.Exec(
		`INSERT INTO provider_configs (`+providerColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Provider, p.MerchantID, p.StoreID, p.AccountID, p.StaticQRIS,
		p.SessionEnc, p.PollIntervalMS, p.ExpiryMS, p.ClockSkewMS, boolToInt(p.Active), toMS(p.CreatedAt), toMS(p.UpdatedAt),
	)
	return err
}

// UpdateProviderConfig updates mutable fields.
func (s *Store) UpdateProviderConfig(p model.ProviderConfig) error {
	_, err := s.db.Exec(
		`UPDATE provider_configs SET name=?, merchant_id=?, store_id=?, account_id=?, static_qris=?,
		 session_enc=?, poll_interval_ms=?, expiry_ms=?, clock_skew_ms=?, active=?, updated_at=? WHERE id=?`,
		p.Name, p.MerchantID, p.StoreID, p.AccountID, p.StaticQRIS,
		p.SessionEnc, p.PollIntervalMS, p.ExpiryMS, p.ClockSkewMS, boolToInt(p.Active), toMS(time.Now()), p.ID,
	)
	return err
}

// UpdateProviderSession stores a freshly persisted upstream session.
func (s *Store) UpdateProviderSession(id, sessionEnc, merchantID, storeID, accountID string) error {
	_, err := s.db.Exec(
		`UPDATE provider_configs SET session_enc=?, merchant_id=?, store_id=?, account_id=?, updated_at=? WHERE id=?`,
		sessionEnc, merchantID, storeID, accountID, toMS(time.Now()), id,
	)
	return err
}

// DeleteProviderConfig removes a config.
func (s *Store) DeleteProviderConfig(id string) error {
	_, err := s.db.Exec(`DELETE FROM provider_configs WHERE id = ?`, id)
	return err
}

// ---- Global settings ----

// GetSettings loads the gateway-wide settings, applying safe defaults.
func (s *Store) GetSettings() (model.GlobalSettings, error) {
	st := model.GlobalSettings{UniqueMin: 1, UniqueMax: 999}
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return st, err
		}
		switch key {
		case "api_token":
			st.APIToken = value
		case "webhook_url":
			st.WebhookURL = value
		case "webhook_secret":
			st.WebhookSecret = value
		case "unique_min":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				st.UniqueMin = n
			}
		case "unique_max":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				st.UniqueMax = n
			}
		case "default_provider_id":
			st.DefaultProviderID = value
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	if st.UniqueMin < 0 {
		st.UniqueMin = 0
	}
	if st.UniqueMax < 1 {
		st.UniqueMax = 999
	}
	if st.UniqueMax < st.UniqueMin {
		st.UniqueMax = st.UniqueMin
	}
	return st, nil
}

// SaveSettings upserts every global setting.
func (s *Store) SaveSettings(st model.GlobalSettings) error {
	now := toMS(time.Now())
	values := map[string]string{
		"api_token":           st.APIToken,
		"webhook_url":         st.WebhookURL,
		"webhook_secret":      st.WebhookSecret,
		"unique_min":          strconv.FormatInt(st.UniqueMin, 10),
		"unique_max":          strconv.FormatInt(st.UniqueMax, 10),
		"default_provider_id": st.DefaultProviderID,
	}
	for key, value := range values {
		if _, err := s.db.Exec(
			`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value, now,
		); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSettings inserts generated defaults without overwriting existing values.
func (s *Store) EnsureSettings(apiToken, webhookSecret string) error {
	now := toMS(time.Now())
	defaults := map[string]string{
		"api_token":           apiToken,
		"webhook_url":         "",
		"webhook_secret":      webhookSecret,
		"unique_min":          "1",
		"unique_max":          "999",
		"default_provider_id": "",
	}
	for key, value := range defaults {
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)`,
			key, value, now,
		); err != nil {
			return err
		}
	}
	return nil
}

// ---- core.PaymentStore ----

func configIDFromPayment(p core.Payment) string {
	if p.Metadata != nil {
		if v, ok := p.Metadata["_config_id"].(string); ok {
			return v
		}
	}
	return ""
}

func (s *Store) resolveConfigID(ctx context.Context, p core.Payment) string {
	if id := configIDFromPayment(p); id != "" {
		return id
	}
	if p.Scope == nil {
		return ""
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM provider_configs WHERE provider=? AND account_id=? AND store_id=? LIMIT 1`,
		p.Scope.Provider, p.Scope.AccountID, p.Scope.MerchantID,
	).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func marshalOrEmpty(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// Create implements core.PaymentStore.
func (s *Store) Create(ctx context.Context, p core.Payment) error {
	provider, account, merchant := "", "", ""
	if p.Scope != nil {
		provider, account, merchant = p.Scope.Provider, p.Scope.AccountID, p.Scope.MerchantID
	}
	var txJSON string
	if p.Transaction != nil {
		txJSON = marshalOrEmpty(p.Transaction)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO payments (id, config_id, reference, base_amount, unique_offset, unique_amount,
		 status, qr_string, provider, account_id, merchant_id, transaction_json, metadata_json,
		 created_at, expires_at, paid_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL)`,
		p.ID, s.resolveConfigID(ctx, p), p.Reference, p.BaseAmount, p.UniqueOffset, p.UniqueAmount,
		string(p.Status), p.QRString, provider, account, merchant, txJSON, marshalOrEmpty(p.Metadata),
		toMS(p.CreatedAt), toMS(p.ExpiresAt),
	)
	return err
}

// Update implements core.PaymentStore.
func (s *Store) Update(ctx context.Context, p core.Payment) error {
	var txJSON string
	if p.Transaction != nil {
		txJSON = marshalOrEmpty(p.Transaction)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE payments SET status=?, qr_string=?, transaction_json=?, metadata_json=?,
		 paid_at = CASE WHEN ? = 'paid' AND paid_at IS NULL THEN ? ELSE paid_at END
		 WHERE id=?`,
		string(p.Status), p.QRString, txJSON, marshalOrEmpty(p.Metadata),
		string(p.Status), toMS(time.Now()), p.ID,
	)
	return err
}

const paymentColumns = `id, config_id, reference, base_amount, unique_offset, unique_amount, status,
	qr_string, provider, account_id, merchant_id, transaction_json, metadata_json,
	created_at, expires_at, paid_at`

func scanPaymentRow(sc interface{ Scan(...any) error }) (*model.PaymentRow, error) {
	var r model.PaymentRow
	var created, expires int64
	var paid sql.NullInt64
	var txJSON, metaJSON string
	if err := sc.Scan(
		&r.ID, &r.ConfigID, &r.Reference, &r.BaseAmount, &r.UniqueOffset, &r.UniqueAmount, &r.Status,
		&r.QRString, &r.Provider, &r.AccountID, &r.MerchantID, &txJSON, &metaJSON,
		&created, &expires, &paid,
	); err != nil {
		return nil, err
	}
	r.Transaction = []byte(txJSON)
	r.Metadata = []byte(metaJSON)
	r.CreatedAt = fromMS(created)
	r.ExpiresAt = fromMS(expires)
	if paid.Valid {
		t := fromMS(paid.Int64)
		r.PaidAt = &t
	}
	return &r, nil
}

// Get implements core.PaymentStore. Returns nil (no error) when missing.
func (s *Store) Get(ctx context.Context, id string) (*core.Payment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = ?`, id)
	r, err := scanPaymentRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p := r.Core()
	return &p, nil
}

// ListActive implements core.PaymentStore.
func (s *Store) ListActive(ctx context.Context, scope *core.PaymentScope) ([]core.Payment, error) {
	query := `SELECT ` + paymentColumns + ` FROM payments WHERE status = 'pending'`
	args := []any{}
	if scope != nil {
		query += ` AND provider = ? AND account_id = ? AND merchant_id = ?`
		args = append(args, scope.Provider, scope.AccountID, scope.MerchantID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Payment
	for rows.Next() {
		r, err := scanPaymentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r.Core())
	}
	return out, rows.Err()
}

// ListQuarantined returns terminal payments whose unique amount was released
// within [since, now]. It lets the allocator stay safe across restarts, when
// the in-memory quarantine is empty.
func (s *Store) ListQuarantined(ctx context.Context, scope *core.PaymentScope, since time.Time) ([]core.Payment, error) {
	query := `SELECT unique_amount, provider, account_id, merchant_id FROM payments
		WHERE status IN ('paid','expired','cancelled')
		AND ((status = 'paid' AND paid_at IS NOT NULL AND paid_at >= ?)
		  OR (status IN ('expired','cancelled') AND expires_at >= ?))`
	args := []any{toMS(since), toMS(since)}
	if scope != nil {
		query += ` AND provider = ? AND account_id = ? AND merchant_id = ?`
		args = append(args, scope.Provider, scope.AccountID, scope.MerchantID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Payment
	for rows.Next() {
		var amount int64
		var provider, account, merchant string
		if err := rows.Scan(&amount, &provider, &account, &merchant); err != nil {
			return nil, err
		}
		p := core.Payment{UniqueAmount: amount}
		if provider != "" {
			p.Scope = &core.PaymentScope{Provider: provider, AccountID: account, MerchantID: merchant}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- Payment queries for the dashboard ----

// PaymentFilter narrows the transaction list.
type PaymentFilter struct {
	ConfigID string
	Status   string
	Query    string
	Page     int
	PageSize int
}

// ListPayments returns a page of payments and the total matching count.
func (s *Store) ListPayments(f PaymentFilter) ([]model.PaymentRow, int, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.ConfigID != "" {
		where = append(where, "config_id = ?")
		args = append(args, f.ConfigID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if strings.TrimSpace(f.Query) != "" {
		where = append(where, "(id LIKE ? OR reference LIKE ?)")
		q := "%" + strings.TrimSpace(f.Query) + "%"
		args = append(args, q, q)
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM payments WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	size := f.PageSize
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.db.Query(
		`SELECT `+paymentColumns+` FROM payments WHERE `+clause+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.PaymentRow
	for rows.Next() {
		r, err := scanPaymentRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

// GetPaymentRow returns a payment row by id.
func (s *Store) GetPaymentRow(id string) (*model.PaymentRow, error) {
	row := s.db.QueryRow(`SELECT `+paymentColumns+` FROM payments WHERE id = ?`, id)
	r, err := scanPaymentRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// Stats returns the dashboard summary.
func (s *Store) Stats() (model.Stats, error) {
	var st model.Stats
	err := s.db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status='paid' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='expired' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='paid' THEN unique_amount ELSE 0 END),0)
		FROM payments`).Scan(
		&st.TotalPayments, &st.PaidPayments, &st.Pending, &st.Expired, &st.Cancelled, &st.PaidVolume,
	)
	if err != nil {
		return st, err
	}
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN session_enc <> '' THEN 1 ELSE 0 END),0)
		FROM provider_configs`).Scan(&st.Providers, &st.Connected)
	return st, err
}

// ---- Webhook deliveries ----

const deliveryColumns = `id, config_id, payment_id, url, payload, signature, status, attempts,
	last_status_code, last_response, next_attempt_at, created_at, updated_at`

func scanDelivery(sc interface{ Scan(...any) error }) (*model.WebhookDelivery, error) {
	var d model.WebhookDelivery
	var payload, signature string
	var next, created, updated int64
	if err := sc.Scan(
		&d.ID, &d.ConfigID, &d.PaymentID, &d.URL, &payload, &signature, &d.Status, &d.Attempts,
		&d.LastStatusCode, &d.LastResponse, &next, &created, &updated,
	); err != nil {
		return nil, err
	}
	d.Payload = []byte(payload)
	d.Signature = signature
	d.NextAttemptAt = fromMS(next)
	d.CreatedAt = fromMS(created)
	d.UpdatedAt = fromMS(updated)
	return &d, nil
}

// CreateDelivery inserts a webhook delivery row.
func (s *Store) CreateDelivery(d model.WebhookDelivery) error {
	if len(d.Payload) == 0 {
		d.Payload = []byte("{}")
	}
	_, err := s.db.Exec(
		`INSERT INTO webhook_deliveries (`+deliveryColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.ConfigID, d.PaymentID, d.URL, string(d.Payload), d.Signature, d.Status, d.Attempts,
		d.LastStatusCode, d.LastResponse, toMS(d.NextAttemptAt), toMS(d.CreatedAt), toMS(d.UpdatedAt),
	)
	return err
}

// UpdateDelivery updates delivery state.
func (s *Store) UpdateDelivery(d model.WebhookDelivery) error {
	_, err := s.db.Exec(
		`UPDATE webhook_deliveries SET status=?, attempts=?, last_status_code=?, last_response=?,
		 next_attempt_at=?, updated_at=? WHERE id=?`,
		d.Status, d.Attempts, d.LastStatusCode, d.LastResponse, toMS(d.NextAttemptAt), toMS(time.Now()), d.ID,
	)
	return err
}

// DueDeliveries returns pending deliveries whose next attempt is due.
func (s *Store) DueDeliveries(now time.Time, limit int) ([]model.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT `+deliveryColumns+` FROM webhook_deliveries
		 WHERE status = 'pending' AND next_attempt_at <= ? ORDER BY next_attempt_at ASC LIMIT ?`,
		toMS(now), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WebhookDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetDelivery loads one delivery.
func (s *Store) GetDelivery(id string) (*model.WebhookDelivery, error) {
	row := s.db.QueryRow(`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE id = ?`, id)
	d, err := scanDelivery(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// DeliveriesByPayment returns the delivery history for a payment.
func (s *Store) DeliveriesByPayment(paymentID string) ([]model.WebhookDelivery, error) {
	rows, err := s.db.Query(
		`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE payment_id = ? ORDER BY created_at DESC`,
		paymentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WebhookDelivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// LatestDelivery returns the most recent delivery for a payment.
func (s *Store) LatestDelivery(paymentID string) (*model.WebhookDelivery, error) {
	row := s.db.QueryRow(
		`SELECT `+deliveryColumns+` FROM webhook_deliveries WHERE payment_id = ? ORDER BY created_at DESC LIMIT 1`,
		paymentID,
	)
	d, err := scanDelivery(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

var _ core.PaymentStore = (*Store)(nil)
