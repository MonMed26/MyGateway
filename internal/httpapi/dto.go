package httpapi

import (
	"encoding/json"
	"time"

	"github.com/MonMed26/MyGateway/core"
	"github.com/MonMed26/MyGateway/internal/model"
)

type providerDTO struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Provider       string    `json:"provider"`
	MerchantID     string    `json:"merchant_id"`
	StoreID        string    `json:"store_id"`
	AccountID      string    `json:"account_id"`
	StaticQRIS     string    `json:"static_qris"`
	PollIntervalMS int64     `json:"poll_interval_ms"`
	ExpiryMS       int64     `json:"expiry_ms"`
	ClockSkewMS    int64     `json:"clock_skew_ms"`
	Active         bool      `json:"active"`
	Connected      bool      `json:"connected"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func toProviderDTO(p model.ProviderConfig) providerDTO {
	return providerDTO{
		ID:             p.ID,
		Name:           p.Name,
		Provider:       p.Provider,
		MerchantID:     p.MerchantID,
		StoreID:        p.StoreID,
		AccountID:      p.AccountID,
		StaticQRIS:     p.StaticQRIS,
		PollIntervalMS: p.PollIntervalMS,
		ExpiryMS:       p.ExpiryMS,
		ClockSkewMS:    p.ClockSkewMS,
		Active:         p.Active,
		Connected:      p.Connected(),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type settingsDTO struct {
	APIToken          string `json:"api_token"`
	WebhookURL        string `json:"webhook_url"`
	WebhookSecret     string `json:"webhook_secret"`
	UniqueMin         int64  `json:"unique_min"`
	UniqueMax         int64  `json:"unique_max"`
	DefaultProviderID string `json:"default_provider_id"`
}

func toSettingsDTO(st model.GlobalSettings) settingsDTO {
	return settingsDTO{
		APIToken:          st.APIToken,
		WebhookURL:        st.WebhookURL,
		WebhookSecret:     st.WebhookSecret,
		UniqueMin:         st.UniqueMin,
		UniqueMax:         st.UniqueMax,
		DefaultProviderID: st.DefaultProviderID,
	}
}

type paymentDTO struct {
	ID           string                    `json:"id"`
	ConfigID     string                    `json:"config_id"`
	Reference    string                    `json:"reference"`
	Status       string                    `json:"status"`
	BaseAmount   int64                     `json:"base_amount"`
	UniqueOffset int64                     `json:"unique_offset"`
	UniqueAmount int64                     `json:"unique_amount"`
	QRString     string                    `json:"qr_string,omitempty"`
	CreatedAt    time.Time                 `json:"created_at"`
	ExpiresAt    time.Time                 `json:"expires_at"`
	PaidAt       *time.Time                `json:"paid_at,omitempty"`
	Transaction  *core.MerchantTransaction `json:"transaction,omitempty"`
	Metadata     map[string]any            `json:"metadata,omitempty"`
}

func toPaymentDTO(r model.PaymentRow) paymentDTO {
	d := paymentDTO{
		ID:           r.ID,
		ConfigID:     r.ConfigID,
		Reference:    r.Reference,
		Status:       r.Status,
		BaseAmount:   r.BaseAmount,
		UniqueOffset: r.UniqueOffset,
		UniqueAmount: r.UniqueAmount,
		QRString:     r.QRString,
		CreatedAt:    r.CreatedAt,
		ExpiresAt:    r.ExpiresAt,
		PaidAt:       r.PaidAt,
	}
	if len(r.Transaction) > 0 {
		var tx core.MerchantTransaction
		if json.Unmarshal(r.Transaction, &tx) == nil {
			d.Transaction = &tx
		}
	}
	if len(r.Metadata) > 0 {
		var meta map[string]any
		if json.Unmarshal(r.Metadata, &meta) == nil {
			delete(meta, "_config_id")
			d.Metadata = meta
		}
	}
	return d
}

type deliveryDTO struct {
	ID             string    `json:"id"`
	ConfigID       string    `json:"config_id"`
	PaymentID      string    `json:"payment_id"`
	URL            string    `json:"url"`
	Status         string    `json:"status"`
	Signature      string    `json:"signature"`
	Attempts       int       `json:"attempts"`
	LastStatusCode int       `json:"last_status_code"`
	LastResponse   string    `json:"last_response"`
	NextAttemptAt  time.Time `json:"next_attempt_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func toDeliveryDTO(d model.WebhookDelivery) deliveryDTO {
	return deliveryDTO{
		ID:             d.ID,
		ConfigID:       d.ConfigID,
		PaymentID:      d.PaymentID,
		URL:            d.URL,
		Status:         d.Status,
		Signature:      d.Signature,
		Attempts:       d.Attempts,
		LastStatusCode: d.LastStatusCode,
		LastResponse:   d.LastResponse,
		NextAttemptAt:  d.NextAttemptAt,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

// clientPaymentDTO is the public, client-facing payment representation.
type clientPaymentDTO struct {
	ID           string    `json:"id"`
	Reference    string    `json:"reference"`
	Status       string    `json:"status"`
	BaseAmount   int64     `json:"base_amount"`
	UniqueOffset int64     `json:"unique_offset"`
	UniqueAmount int64     `json:"unique_amount"`
	QRString     string    `json:"qr_string,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func toClientPaymentDTO(r model.PaymentRow) clientPaymentDTO {
	return clientPaymentDTO{
		ID:           r.ID,
		Reference:    r.Reference,
		Status:       r.Status,
		BaseAmount:   r.BaseAmount,
		UniqueOffset: r.UniqueOffset,
		UniqueAmount: r.UniqueAmount,
		QRString:     r.QRString,
		CreatedAt:    r.CreatedAt,
		ExpiresAt:    r.ExpiresAt,
	}
}
