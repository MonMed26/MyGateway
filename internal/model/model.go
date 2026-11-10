// Package model holds the gateway's persisted domain types.
package model

import (
	"encoding/json"
	"time"

	"github.com/hirotomasato/paygateme/core"
)

// Provider identifiers.
const (
	ProviderShopee = "shopee"
	ProviderGoPay  = "gopay"
)

// Statuses for provider configurations.
const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
)

// Webhook delivery statuses.
const (
	DeliveryPending = "pending"
	DeliverySuccess = "success"
	DeliveryFailed  = "failed"
)

// ProviderConfig is a configured upstream merchant account exposed through the
// gateway. Client credentials, webhook settings and the unique-code range are
// global (see GlobalSettings), not per provider.
type ProviderConfig struct {
	ID             string
	Name           string
	Provider       string
	MerchantID     string
	StoreID        string
	AccountID      string
	StaticQRIS     string
	SessionEnc     string
	PollIntervalMS int64
	ExpiryMS       int64
	ClockSkewMS    int64
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GlobalSettings holds the gateway-wide client credentials and payment policy.
type GlobalSettings struct {
	// APIToken is the single Bearer token clients use to call the gateway.
	APIToken string
	// WebhookURL and WebhookSecret configure outbound payment notifications.
	WebhookURL    string
	WebhookSecret string
	// UniqueMin/UniqueMax define the global unique-code window.
	UniqueMin int64
	UniqueMax int64
	// DefaultProviderID is used when a client omits provider_id.
	DefaultProviderID string
}

// Connected reports whether an upstream session has been stored.
func (p ProviderConfig) Connected() bool { return p.SessionEnc != "" }

// AdminUser is a dashboard administrator.
type AdminUser struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// PaymentRow is a payment joined with the config it belongs to.
type PaymentRow struct {
	ID           string
	ConfigID     string
	Reference    string
	BaseAmount   int64
	UniqueOffset int64
	UniqueAmount int64
	Status       string
	QRString     string
	Provider     string
	AccountID    string
	MerchantID   string
	Transaction  []byte
	Metadata     []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
	PaidAt       *time.Time
}

// Core converts the row into the provider-neutral core.Payment type.
func (r PaymentRow) Core() core.Payment {
	p := core.Payment{
		ID:           r.ID,
		BaseAmount:   r.BaseAmount,
		UniqueOffset: r.UniqueOffset,
		UniqueAmount: r.UniqueAmount,
		Status:       core.PaymentStatus(r.Status),
		CreatedAt:    r.CreatedAt,
		ExpiresAt:    r.ExpiresAt,
		Reference:    r.Reference,
		QRString:     r.QRString,
	}
	if r.Provider != "" {
		p.Scope = &core.PaymentScope{Provider: r.Provider, AccountID: r.AccountID, MerchantID: r.MerchantID}
	}
	if len(r.Transaction) > 0 {
		var tx core.MerchantTransaction
		if json.Unmarshal(r.Transaction, &tx) == nil {
			p.Transaction = &tx
		}
	}
	if len(r.Metadata) > 0 {
		var meta map[string]any
		if json.Unmarshal(r.Metadata, &meta) == nil {
			p.Metadata = meta
		}
	}
	return p
}

// WebhookDelivery is one outbound webhook attempt series.
type WebhookDelivery struct {
	ID             string
	ConfigID       string
	PaymentID      string
	URL            string
	Payload        []byte
	Signature      string
	Status         string
	Attempts       int
	LastStatusCode int
	LastResponse   string
	NextAttemptAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Stats is the dashboard summary.
type Stats struct {
	TotalPayments int64 `json:"total_payments"`
	PaidPayments  int64 `json:"paid_payments"`
	Pending       int64 `json:"pending"`
	Expired       int64 `json:"expired"`
	Cancelled     int64 `json:"cancelled"`
	PaidVolume    int64 `json:"paid_volume"`
	Providers     int64 `json:"providers"`
	Connected     int64 `json:"connected"`
}
