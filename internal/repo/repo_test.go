package repo_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MonMed26/MyGateway/core"
	"github.com/MonMed26/MyGateway/internal/db"
	"github.com/MonMed26/MyGateway/internal/model"
	"github.com/MonMed26/MyGateway/internal/repo"
)

func newStore(t *testing.T) *repo.Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return repo.New(conn)
}

func createProvider(t *testing.T, s *repo.Store) model.ProviderConfig {
	t.Helper()
	now := time.Now()
	cfg := model.ProviderConfig{
		ID: "cfg1", Name: "Toko", Provider: model.ProviderShopee,
		PollIntervalMS: 3000, ExpiryMS: 300000, ClockSkewMS: 60000,
		Active: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateProviderConfig(cfg); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return cfg
}

func TestProviderConfigLifecycle(t *testing.T) {
	s := newStore(t)
	cfg := createProvider(t, s)

	got, err := s.GetProviderConfig(cfg.ID)
	if err != nil || got == nil {
		t.Fatalf("get provider: %v", err)
	}
	if got.Name != cfg.Name || got.Provider != cfg.Provider {
		t.Fatalf("unexpected provider: %+v", got)
	}
	if got.Connected() {
		t.Fatal("provider should not be connected yet")
	}

	if err := s.UpdateProviderSession(cfg.ID, "enc-session", "merchant-1", "store-1", "acct-1"); err != nil {
		t.Fatalf("update session: %v", err)
	}
	got, _ = s.GetProviderConfig(cfg.ID)
	if !got.Connected() || got.StoreID != "store-1" {
		t.Fatalf("session not persisted: %+v", got)
	}
}

func TestGlobalSettings(t *testing.T) {
	s := newStore(t)

	if err := s.EnsureSettings("initial-token", "initial-secret"); err != nil {
		t.Fatalf("ensure settings: %v", err)
	}
	st, err := s.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if st.APIToken != "initial-token" || st.WebhookSecret != "initial-secret" || st.UniqueMax != 999 {
		t.Fatalf("unexpected defaults: %+v", st)
	}

	st.APIToken = "rotated"
	st.WebhookURL = "https://example.com/hook"
	st.UniqueMin = 10
	st.UniqueMax = 250
	if err := s.SaveSettings(st); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	got, _ := s.GetSettings()
	if got.APIToken != "rotated" || got.WebhookURL != "https://example.com/hook" || got.UniqueMin != 10 || got.UniqueMax != 250 {
		t.Fatalf("settings not persisted: %+v", got)
	}

	// EnsureSettings must not clobber existing values.
	if err := s.EnsureSettings("other", "other"); err != nil {
		t.Fatalf("ensure settings again: %v", err)
	}
	got, _ = s.GetSettings()
	if got.APIToken != "rotated" {
		t.Fatalf("EnsureSettings clobbered existing token: %+v", got)
	}
}

func TestPaymentStore(t *testing.T) {
	s := newStore(t)
	cfg := createProvider(t, s)
	ctx := context.Background()
	now := time.Now()
	scope := &core.PaymentScope{Provider: model.ProviderShopee, AccountID: "acct-1", MerchantID: "store-1"}

	p := core.Payment{
		ID:           "pay1",
		Scope:        scope,
		BaseAmount:   10000,
		UniqueOffset: 5,
		UniqueAmount: 10005,
		Status:       core.PaymentPending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		Reference:    "ORDER-1",
		Metadata:     map[string]any{"_config_id": cfg.ID, "order_id": "A-1"},
	}
	if err := s.Create(ctx, p); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	loaded, err := s.Get(ctx, "pay1")
	if err != nil || loaded == nil {
		t.Fatalf("get payment: %v", err)
	}
	if loaded.UniqueAmount != 10005 || loaded.Scope == nil || loaded.Scope.MerchantID != "store-1" {
		t.Fatalf("unexpected payment: %+v", loaded)
	}
	if loaded.Metadata["order_id"] != "A-1" {
		t.Fatalf("metadata not persisted: %+v", loaded.Metadata)
	}

	active, err := s.ListActive(ctx, scope)
	if err != nil || len(active) != 1 {
		t.Fatalf("list active: %v len=%d", err, len(active))
	}
	other, _ := s.ListActive(ctx, &core.PaymentScope{Provider: model.ProviderShopee, AccountID: "x", MerchantID: "y"})
	if len(other) != 0 {
		t.Fatalf("scope filter leaked %d payments", len(other))
	}

	// Settle it.
	loaded.Status = core.PaymentPaid
	loaded.Transaction = &core.MerchantTransaction{ID: "tx1", Amount: 10005, Status: "settlement", Time: now}
	if err := s.Update(ctx, *loaded); err != nil {
		t.Fatalf("update payment: %v", err)
	}

	row, err := s.GetPaymentRow("pay1")
	if err != nil || row == nil {
		t.Fatalf("get row: %v", err)
	}
	if row.Status != string(core.PaymentPaid) || row.PaidAt == nil {
		t.Fatalf("payment not marked paid: %+v", row)
	}

	rows, total, err := s.ListPayments(repo.PaymentFilter{ConfigID: cfg.ID, Status: "paid"})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("list payments: %v total=%d len=%d", err, total, len(rows))
	}

	quarantined, err := s.ListQuarantined(ctx, scope, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("list quarantined: %v", err)
	}
	if len(quarantined) != 1 || quarantined[0].UniqueAmount != 10005 {
		t.Fatalf("unexpected quarantined payments: %+v", quarantined)
	}

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalPayments != 1 || stats.PaidPayments != 1 || stats.PaidVolume != 10005 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestWebhookDeliveries(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	d := model.WebhookDelivery{
		ID: "d1", ConfigID: "cfg1", PaymentID: "pay1", URL: "https://example.com/hook",
		Payload: []byte(`{"event":"payment.paid"}`), Signature: "sig",
		Status: model.DeliveryPending, NextAttemptAt: now.Add(-time.Second),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateDelivery(d); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	due, err := s.DueDeliveries(now, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due deliveries: %v len=%d", err, len(due))
	}
	d.Attempts = 1
	d.Status = model.DeliverySuccess
	if err := s.UpdateDelivery(d); err != nil {
		t.Fatalf("update delivery: %v", err)
	}
	latest, err := s.LatestDelivery("pay1")
	if err != nil || latest == nil || latest.Status != model.DeliverySuccess {
		t.Fatalf("latest delivery: %v %+v", err, latest)
	}
}
