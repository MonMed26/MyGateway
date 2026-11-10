package gateway_test

import (
	"encoding/json"
	"testing"

	"github.com/hirotomasato/paygateme/core"
	"github.com/hirotomasato/paygateme/internal/gateway"
	"github.com/hirotomasato/paygateme/internal/model"
)

func TestEventPayload(t *testing.T) {
	cfg := model.ProviderConfig{ID: "cfg1"}
	p := core.Payment{
		ID:           "pay1",
		Status:       core.PaymentPaid,
		BaseAmount:   10000,
		UniqueOffset: 2,
		UniqueAmount: 10002,
		Reference:    "ORDER-1",
		Metadata:     map[string]any{"_config_id": "cfg1", "order_id": "A-1"},
	}
	raw := gateway.EventPayload(cfg, p)

	var env struct {
		Event   string `json:"event"`
		Payment struct {
			ID       string         `json:"id"`
			Status   string         `json:"status"`
			Metadata map[string]any `json:"metadata"`
		} `json:"payment"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Event != "payment.paid" {
		t.Fatalf("unexpected event %q", env.Event)
	}
	if env.Payment.ID != "pay1" || env.Payment.Status != "paid" {
		t.Fatalf("unexpected payment: %+v", env.Payment)
	}
	if _, exists := env.Payment.Metadata["_config_id"]; exists {
		t.Fatal("internal _config_id leaked into webhook metadata")
	}
	if env.Payment.Metadata["order_id"] != "A-1" {
		t.Fatalf("user metadata missing: %+v", env.Payment.Metadata)
	}
}
