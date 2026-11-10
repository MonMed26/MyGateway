// Package gateway wires the provider adapters to the SQLite store, runs their
// payment services, and emits webhook events on settlement.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hirotomasato/paygateme/core"
	"github.com/hirotomasato/paygateme/gopay"
	"github.com/hirotomasato/paygateme/internal/model"
	"github.com/hirotomasato/paygateme/internal/repo"
	"github.com/hirotomasato/paygateme/internal/security"
	"github.com/hirotomasato/paygateme/internal/webhook"
	"github.com/hirotomasato/paygateme/payment"
	"github.com/hirotomasato/paygateme/shopee"
	"github.com/hirotomasato/paygateme/utils"
)

// Manager owns live provider instances keyed by config id.
type Manager struct {
	repo       *repo.Store
	masterKey  string
	logger     utils.Logger
	dispatcher *webhook.Dispatcher

	mu            sync.Mutex
	settings      model.GlobalSettings
	instances     map[string]*Instance
	challenges    map[string]*shopee.OtpChallenge
	verifications map[string]*shopee.OtpVerification
}

// Instance is one configured provider account.
type Instance struct {
	mu       sync.Mutex
	cfg      model.ProviderConfig
	shopee   *shopee.Provider
	gopay    *gopay.Provider
	service  *payment.Service
	handlers bool
}

// NewManager constructs a manager.
func NewManager(r *repo.Store, masterKey string, logger utils.Logger, dispatcher *webhook.Dispatcher) *Manager {
	if logger == nil {
		logger = utils.NoopLogger
	}
	m := &Manager{
		repo:          r,
		masterKey:     masterKey,
		logger:        logger,
		dispatcher:    dispatcher,
		instances:     make(map[string]*Instance),
		challenges:    make(map[string]*shopee.OtpChallenge),
		verifications: make(map[string]*shopee.OtpVerification),
	}
	if st, err := r.GetSettings(); err == nil {
		m.settings = st
	}
	return m
}

// ReloadSettings refreshes the cached global settings.
func (m *Manager) ReloadSettings() {
	st, err := m.repo.GetSettings()
	if err != nil {
		m.logger.Error("gateway: reload settings failed", map[string]any{"error": err.Error()})
		return
	}
	m.mu.Lock()
	m.settings = st
	m.mu.Unlock()
}

// RebuildAll restarts every provider service, e.g. after global settings change.
func (m *Manager) RebuildAll() {
	configs, err := m.repo.ListProviderConfigs()
	if err != nil {
		m.logger.Error("gateway: list configs for rebuild failed", map[string]any{"error": err.Error()})
		return
	}
	for _, cfg := range configs {
		if err := m.Rebuild(cfg.ID); err != nil {
			m.logger.Warn("gateway: rebuild failed", map[string]any{"config": cfg.ID, "error": err.Error()})
		}
	}
}

// StartAll rebuilds and starts every active, connected config.
func (m *Manager) StartAll(ctx context.Context) {
	configs, err := m.repo.ListProviderConfigs()
	if err != nil {
		m.logger.Error("gateway: list configs failed", map[string]any{"error": err.Error()})
		return
	}
	for _, cfg := range configs {
		if !cfg.Active || !cfg.Connected() {
			continue
		}
		if err := m.Rebuild(cfg.ID); err != nil {
			m.logger.Warn("gateway: start config failed", map[string]any{
				"config": cfg.ID, "provider": cfg.Provider, "error": err.Error(),
			})
		}
	}
}

// Close stops all running payment services.
func (m *Manager) Close() {
	m.mu.Lock()
	instances := make([]*Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.instances = make(map[string]*Instance)
	m.mu.Unlock()
	for _, inst := range instances {
		inst.mu.Lock()
		if inst.service != nil {
			inst.service.Stop()
		}
		inst.mu.Unlock()
	}
}

// Rebuild discards any cached instance for id, then rebuilds and starts it when
// the config is active and has a stored session.
func (m *Manager) Rebuild(id string) error {
	m.mu.Lock()
	if old := m.instances[id]; old != nil {
		delete(m.instances, id)
		old.mu.Lock()
		if old.service != nil {
			old.service.Stop()
		}
		old.mu.Unlock()
	}
	m.mu.Unlock()

	cfg, err := m.repo.GetProviderConfig(id)
	if err != nil {
		return err
	}
	if cfg == nil || !cfg.Active || !cfg.Connected() {
		return nil
	}
	return m.buildAndStart(*cfg)
}

// rebuildAfterLogin rebuilds from the freshly persisted session.
func (m *Manager) rebuildAfterLogin(id string) error {
	return m.Rebuild(id)
}

func (m *Manager) buildAndStart(cfg model.ProviderConfig) error {
	m.mu.Lock()
	inst, err := m.buildInstanceLocked(cfg)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.instances[cfg.ID] = inst
	m.mu.Unlock()

	// Starting can legitimately fail before a store/outlet is selected; that is
	// not fatal, the admin will finish setup and trigger another rebuild.
	if err := m.startInstance(inst); err != nil {
		m.logger.Warn("gateway: payment service not started yet", map[string]any{
			"config": cfg.ID, "error": err.Error(),
		})
	}
	return nil
}

func (m *Manager) buildInstanceLocked(cfg model.ProviderConfig) (*Instance, error) {
	inst := &Instance{cfg: cfg}

	var session []byte
	if cfg.SessionEnc != "" {
		b, err := security.DecryptSession(cfg.SessionEnc, m.masterKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt provider session: %w", err)
		}
		session = b
	}

	switch cfg.Provider {
	case model.ProviderShopee:
		var s shopee.Session
		var sptr *shopee.Session
		if len(session) > 0 {
			if err := json.Unmarshal(session, &s); err != nil {
				return nil, fmt.Errorf("decode shopee session: %w", err)
			}
			sptr = &s
		}
		inst.shopee = shopee.NewProvider(shopee.ProviderConfig{
			Session:       sptr,
			Store:         m.repo,
			StaticQris:    cfg.StaticQRIS,
			DeviceReport:  shopee.DeviceRiskBlob,
			PollInterval:  cfg.PollIntervalMS,
			DefaultExpiry: cfg.ExpiryMS,
			ClockSkew:     cfg.ClockSkewMS,
			UniqueMin:     m.settings.UniqueMin,
			UniqueMax:     m.settings.UniqueMax,
			Logger:        m.logger,
			OnSessionUpdated: func(sess shopee.Session) error {
				return m.saveShopeeSession(cfg.ID, sess)
			},
		})
	case model.ProviderGoPay:
		var s gopay.SessionState
		var sptr *gopay.SessionState
		if len(session) > 0 {
			if err := json.Unmarshal(session, &s); err != nil {
				return nil, fmt.Errorf("decode gopay session: %w", err)
			}
			sptr = &s
		}
		inst.gopay = gopay.NewProvider(gopay.ProviderConfig{
			Session:       sptr,
			StaticQRIS:    cfg.StaticQRIS,
			Store:         m.repo,
			PollInterval:  cfg.PollIntervalMS,
			DefaultExpiry: cfg.ExpiryMS,
			ClockSkew:     cfg.ClockSkewMS,
			UniqueMin:     m.settings.UniqueMin,
			UniqueMax:     m.settings.UniqueMax,
			Logger:        m.logger,
			OnSessionUpdated: func(sess gopay.SessionState) error {
				return m.saveGoPaySession(cfg.ID, sess)
			},
			OnTokenRefreshed: func(sess gopay.SessionState) error {
				return m.saveGoPaySession(cfg.ID, sess)
			},
		})
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	return inst, nil
}

func (inst *Instance) payments() (*payment.Service, error) {
	if inst.shopee != nil {
		return inst.shopee.Payments()
	}
	if inst.gopay != nil {
		return inst.gopay.Payments()
	}
	return nil, fmt.Errorf("provider instance is not initialized")
}

func (m *Manager) startInstance(inst *Instance) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	svc, err := inst.payments()
	if err != nil {
		return err
	}
	if !inst.handlers {
		cfgID := inst.cfg.ID
		svc.OnPaid(func(p core.Payment) { m.onPaid(cfgID, p) })
		svc.OnError(func(e error) {
			m.logger.Error("gateway: payment service error", map[string]any{"config": cfgID, "error": e.Error()})
		})
		inst.handlers = true
	}
	inst.service = svc
	svc.Start()
	return nil
}

// Service returns a running payment service for a config.
func (m *Manager) Service(id string) (*payment.Service, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	inst.mu.Lock()
	svc := inst.service
	inst.mu.Unlock()
	if svc == nil || !svc.IsRunning() {
		if err := m.startInstance(inst); err != nil {
			return nil, err
		}
		inst.mu.Lock()
		svc = inst.service
		inst.mu.Unlock()
	}
	return svc, nil
}

func (m *Manager) instance(id string, requireSession bool) (*Instance, error) {
	m.mu.Lock()
	if inst := m.instances[id]; inst != nil {
		m.mu.Unlock()
		return inst, nil
	}
	m.mu.Unlock()

	cfg, err := m.repo.GetProviderConfig(id)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("provider config not found")
	}
	if requireSession && !cfg.Connected() {
		return nil, fmt.Errorf("provider is not connected; complete OTP login first")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if inst := m.instances[id]; inst != nil {
		return inst, nil
	}
	inst, err := m.buildInstanceLocked(*cfg)
	if err != nil {
		return nil, err
	}
	m.instances[id] = inst
	return inst, nil
}

// ---- Session persistence ----

func (m *Manager) saveShopeeSession(id string, s shopee.Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	enc, err := security.EncryptSession(data, m.masterKey)
	if err != nil {
		return err
	}
	merchantID, storeID := s.Merchant.ID, s.StoreID
	if err := m.repo.UpdateProviderSession(id, enc, merchantID, storeID, s.AccountID); err != nil {
		return err
	}
	m.mu.Lock()
	if inst := m.instances[id]; inst != nil {
		inst.cfg.SessionEnc = enc
		inst.cfg.MerchantID = merchantID
		inst.cfg.StoreID = storeID
		inst.cfg.AccountID = s.AccountID
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) saveGoPaySession(id string, s gopay.SessionState) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	enc, err := security.EncryptSession(data, m.masterKey)
	if err != nil {
		return err
	}
	merchantID, outletID := "", ""
	if s.Merchant != nil {
		merchantID = s.Merchant.ID
	}
	outletID = s.OutletID
	if err := m.repo.UpdateProviderSession(id, enc, merchantID, outletID, merchantID); err != nil {
		return err
	}
	m.mu.Lock()
	if inst := m.instances[id]; inst != nil {
		inst.cfg.SessionEnc = enc
		inst.cfg.MerchantID = merchantID
		inst.cfg.StoreID = outletID
		inst.cfg.AccountID = merchantID
	}
	m.mu.Unlock()
	return nil
}

// ---- OTP / setup flows ----

// RequestShopeeOtp starts the Shopee OTP flow and remembers the challenge.
func (m *Manager) RequestShopeeOtp(ctx context.Context, id, phone, password, deviceReport string) (*shopee.OtpChallenge, error) {
	inst, err := m.instance(id, false)
	if err != nil {
		return nil, err
	}
	if inst.shopee == nil {
		return nil, fmt.Errorf("config is not a Shopee provider")
	}
	ch, err := inst.shopee.RequestOtp(ctx, phone, shopee.OtpRequestOptions{
		Password:     password,
		DeviceReport: deviceReport,
	})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.challenges[id] = ch
	m.mu.Unlock()
	return ch, nil
}

// VerifyShopeeOtp completes (or advances) the Shopee login.
func (m *Manager) VerifyShopeeOtp(ctx context.Context, id, otp string) (*shopee.LoginOutcome, error) {
	m.mu.Lock()
	ch := m.challenges[id]
	m.mu.Unlock()
	if ch == nil {
		return nil, fmt.Errorf("no pending OTP challenge; request a new code")
	}
	inst, err := m.instance(id, false)
	if err != nil {
		return nil, err
	}
	outcome, err := inst.shopee.LoginWithOtp(ctx, shopee.LoginWithOtpInput{
		Challenge: *ch,
		OTP:       otp,
	})
	if err != nil {
		m.logger.Error("gateway: shopee otp verification failed", map[string]any{
			"config": id,
			"error":  err.Error(),
		})
		return nil, err
	}
	m.mu.Lock()
	delete(m.challenges, id)
	if outcome.Status == shopee.LoginMerchantSelectionNeeded {
		v := outcome.Verification
		m.verifications[id] = v
	}
	m.mu.Unlock()
	if outcome.Status == shopee.LoginComplete {
		if err := m.rebuildAfterLogin(id); err != nil {
			return nil, err
		}
	}
	return outcome, nil
}

// CompleteShopeeLogin finishes a login that required merchant selection.
func (m *Manager) CompleteShopeeLogin(ctx context.Context, id, merchantID string) (*shopee.Session, error) {
	m.mu.Lock()
	v := m.verifications[id]
	m.mu.Unlock()
	if v == nil {
		return nil, fmt.Errorf("no pending merchant selection; restart the login")
	}
	inst, err := m.instance(id, false)
	if err != nil {
		return nil, err
	}
	sess, err := inst.shopee.CompleteLogin(ctx, shopee.CompleteLoginInput{
		Verification: *v,
		MerchantID:   merchantID,
	})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.verifications, id)
	m.mu.Unlock()
	if err := m.rebuildAfterLogin(id); err != nil {
		return nil, err
	}
	return sess, nil
}

// RequestGoPayOtp starts the GoPay OTP flow.
func (m *Manager) RequestGoPayOtp(ctx context.Context, id, phone, countryCode string) (*gopay.LoginRequestResult, error) {
	inst, err := m.instance(id, false)
	if err != nil {
		return nil, err
	}
	if inst.gopay == nil {
		return nil, fmt.Errorf("config is not a GoPay provider")
	}
	return inst.gopay.RequestOTP(ctx, phone, countryCode)
}

// VerifyGoPayOtp completes the GoPay login.
func (m *Manager) VerifyGoPayOtp(ctx context.Context, id, otp, otpToken string) (*gopay.SessionState, error) {
	inst, err := m.instance(id, false)
	if err != nil {
		return nil, err
	}
	sess, err := inst.gopay.VerifyOTP(ctx, otp, otpToken)
	if err != nil {
		m.logger.Error("gateway: gopay otp verification failed", map[string]any{
			"config": id,
			"error":  err.Error(),
		})
		return nil, err
	}
	if err := m.rebuildAfterLogin(id); err != nil {
		return nil, err
	}
	return sess, nil
}

// SelectShopeeStore selects a store and restarts the payment service.
func (m *Manager) SelectShopeeStore(ctx context.Context, id, storeID string) (*shopee.Session, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	sess, err := inst.shopee.SelectStore(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if err := m.Rebuild(id); err != nil {
		return nil, err
	}
	return sess, nil
}

// SelectShopeeMerchant switches merchant and restarts the payment service.
func (m *Manager) SelectShopeeMerchant(ctx context.Context, id, merchantID string) (*shopee.Session, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	sess, err := inst.shopee.SelectMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	if err := m.Rebuild(id); err != nil {
		return nil, err
	}
	return sess, nil
}

// SelectGoPayMerchant switches merchant.
func (m *Manager) SelectGoPayMerchant(ctx context.Context, id, merchantID string) (*gopay.StoredMerchant, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	merchant, err := inst.gopay.SelectMerchant(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	if err := m.Rebuild(id); err != nil {
		return nil, err
	}
	return merchant, nil
}

// SelectGoPayOutlet selects an outlet.
func (m *Manager) SelectGoPayOutlet(ctx context.Context, id, popID string) (*gopay.MerchantOutlet, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	outlet, err := inst.gopay.SelectOutlet(ctx, popID)
	if err != nil {
		return nil, err
	}
	if err := m.Rebuild(id); err != nil {
		return nil, err
	}
	return outlet, nil
}

// Refresh refreshes the upstream session without a new OTP.
func (m *Manager) Refresh(ctx context.Context, id string) error {
	inst, err := m.instance(id, true)
	if err != nil {
		return err
	}
	switch {
	case inst.shopee != nil:
		if _, err := inst.shopee.RefreshSession(ctx); err != nil {
			return err
		}
	case inst.gopay != nil:
		if _, err := inst.gopay.RefreshSession(ctx); err != nil {
			return err
		}
	}
	return m.Rebuild(id)
}

// SetQRIS binds a static QRIS payload and rebuilds the service.
func (m *Manager) SetQRIS(ctx context.Context, id, payload string) error {
	inst, err := m.instance(id, true)
	if err != nil {
		return err
	}
	switch {
	case inst.shopee != nil:
		if err := inst.shopee.SetStaticQris(payload); err != nil {
			return err
		}
	case inst.gopay != nil:
		if err := inst.gopay.SetStaticQRIS(payload); err != nil {
			return err
		}
	}
	cfg, err := m.repo.GetProviderConfig(id)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("provider config not found")
	}
	cfg.StaticQRIS = payload
	if err := m.repo.UpdateProviderConfig(*cfg); err != nil {
		return err
	}
	return m.Rebuild(id)
}

// ShopeeStores lists the stores on the active Shopee merchant.
func (m *Manager) ShopeeStores(ctx context.Context, id string) ([]shopee.Store, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	if inst.shopee == nil {
		return nil, fmt.Errorf("config is not a Shopee provider")
	}
	return inst.shopee.ListStores(ctx)
}

// GoPayMerchants lists GoPay merchants.
func (m *Manager) GoPayMerchants(ctx context.Context, id string) ([]gopay.StoredMerchant, error) {
	inst, err := m.instance(id, true)
	if err != nil {
		return nil, err
	}
	if inst.gopay == nil {
		return nil, fmt.Errorf("config is not a GoPay provider")
	}
	return inst.gopay.ListMerchants(ctx)
}

// Status is a non-secret snapshot of a provider config.
type Status struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Connected     bool   `json:"connected"`
	Authenticated bool   `json:"authenticated"`
	MerchantID    string `json:"merchant_id"`
	StoreID       string `json:"store_id"`
	AccountID     string `json:"account_id"`
	MerchantName  string `json:"merchant_name,omitempty"`
	StoreName     string `json:"store_name,omitempty"`
	OutletID      string `json:"outlet_id,omitempty"`
	HasQRIS       bool   `json:"has_qris"`
	Polling       bool   `json:"polling"`
	Error         string `json:"error,omitempty"`
}

// Status returns the live status for a config.
func (m *Manager) Status(id string) (*Status, error) {
	cfg, err := m.repo.GetProviderConfig(id)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("provider config not found")
	}
	st := &Status{
		ID:         cfg.ID,
		Name:       cfg.Name,
		Provider:   cfg.Provider,
		Connected:  cfg.Connected(),
		MerchantID: cfg.MerchantID,
		StoreID:    cfg.StoreID,
		AccountID:  cfg.AccountID,
		HasQRIS:    cfg.StaticQRIS != "",
	}
	inst, err := m.instance(id, false)
	if err != nil {
		st.Error = err.Error()
		return st, nil
	}
	inst.mu.Lock()
	if inst.service != nil {
		st.Polling = inst.service.IsRunning()
	}
	inst.mu.Unlock()

	if inst.shopee != nil {
		st.Authenticated = inst.shopee.Authenticated()
		if sess := inst.shopee.ExportSession(); sess != nil {
			st.MerchantName = sess.Merchant.Name
			st.MerchantID = sess.Merchant.ID
			st.StoreID = sess.StoreID
			st.AccountID = sess.AccountID
			for _, s := range sess.Stores {
				if s.ID == sess.StoreID {
					st.StoreName = s.Name
					break
				}
			}
		}
	}
	if inst.gopay != nil {
		st.Authenticated = inst.gopay.Authenticated()
		if sess := inst.gopay.ExportSession(); sess != nil {
			if sess.Merchant != nil {
				st.MerchantName = sess.Merchant.MerchantName
				st.MerchantID = sess.Merchant.ID
			}
			st.OutletID = sess.OutletID
		}
	}
	return st, nil
}

// ---- Webhook event ----

func (m *Manager) onPaid(configID string, p core.Payment) {
	cfg, err := m.repo.GetProviderConfig(configID)
	if err != nil || cfg == nil {
		m.logger.Error("gateway: load config for webhook failed", map[string]any{"config": configID})
		return
	}
	payload := EventPayload(*cfg, p)
	if _, err := m.dispatcher.Enqueue(configID, p.ID, payload); err != nil {
		m.logger.Error("gateway: enqueue webhook failed", map[string]any{"payment": p.ID, "error": err.Error()})
	}
}

// EventPayload builds the signed-JSON webhook body for a paid payment.
func EventPayload(cfg model.ProviderConfig, p core.Payment) []byte {
	meta := make(map[string]any, len(p.Metadata))
	for k, v := range p.Metadata {
		if k == "_config_id" {
			continue
		}
		meta[k] = v
	}
	now := time.Now().UTC()
	env := struct {
		Event     string         `json:"event"`
		CreatedAt time.Time      `json:"created_at"`
		Payment   webhookPayment `json:"payment"`
	}{
		Event:     "payment.paid",
		CreatedAt: now,
		Payment: webhookPayment{
			ID:           p.ID,
			ConfigID:     cfg.ID,
			Reference:    p.Reference,
			BaseAmount:   p.BaseAmount,
			UniqueOffset: p.UniqueOffset,
			UniqueAmount: p.UniqueAmount,
			Status:       string(p.Status),
			QRString:     p.QRString,
			CreatedAt:    p.CreatedAt,
			ExpiresAt:    p.ExpiresAt,
			PaidAt:       now,
			Transaction:  p.Transaction,
			Metadata:     meta,
		},
	}
	b, _ := json.Marshal(env)
	return b
}

type webhookPayment struct {
	ID           string                    `json:"id"`
	ConfigID     string                    `json:"config_id"`
	Reference    string                    `json:"reference"`
	BaseAmount   int64                     `json:"base_amount"`
	UniqueOffset int64                     `json:"unique_offset"`
	UniqueAmount int64                     `json:"unique_amount"`
	Status       string                    `json:"status"`
	QRString     string                    `json:"qr_string,omitempty"`
	CreatedAt    time.Time                 `json:"created_at"`
	ExpiresAt    time.Time                 `json:"expires_at"`
	PaidAt       time.Time                 `json:"paid_at"`
	Transaction  *core.MerchantTransaction `json:"transaction,omitempty"`
	Metadata     map[string]any            `json:"metadata,omitempty"`
}
