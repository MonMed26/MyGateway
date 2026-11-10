package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hirotomasato/paygateme/core"
	"github.com/hirotomasato/paygateme/internal/model"
	"github.com/hirotomasato/paygateme/payment"
)

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.repo.GetSettings()
	configs, _ := s.repo.ListProviderConfigs()
	connected := 0
	for _, cfg := range configs {
		if cfg.Active && cfg.Connected() {
			connected++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"webhook_configured": settings.WebhookURL != "" && settings.WebhookSecret != "",
		"providers":          len(configs),
		"connected":          connected,
		"time":               time.Now().UTC(),
	})
}

// resolveClientProvider picks the provider config for a client request. The
// request may name one explicitly; otherwise the global default is used, and
// finally the only connected provider when exactly one exists.
func (s *Server) resolveClientProvider(requested string) (*model.ProviderConfig, error) {
	settings, err := s.repo.GetSettings()
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(requested)
	if id == "" {
		id = settings.DefaultProviderID
	}
	if id != "" {
		cfg, err := s.repo.GetProviderConfig(id)
		if err != nil {
			return nil, err
		}
		if cfg == nil {
			return nil, errors.New("provider_id not found")
		}
		if !cfg.Active {
			return nil, errors.New("provider is inactive")
		}
		if !cfg.Connected() {
			return nil, errors.New("provider is not connected; complete OTP login first")
		}
		return cfg, nil
	}

	configs, err := s.repo.ListProviderConfigs()
	if err != nil {
		return nil, err
	}
	candidates := make([]model.ProviderConfig, 0, len(configs))
	for _, cfg := range configs {
		if cfg.Active && cfg.Connected() {
			candidates = append(candidates, cfg)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, errors.New("no connected provider; connect one in the dashboard first")
	case 1:
		return &candidates[0], nil
	default:
		return nil, errors.New("provider_id is required when multiple providers are connected")
	}
}

func (s *Server) handleClientCreatePayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProviderID       string         `json:"provider_id"`
		Amount           int64          `json:"amount"`
		Reference        string         `json:"reference"`
		ExpiresInSeconds int64          `json:"expires_in_seconds"`
		Metadata         map[string]any `json:"metadata"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be a positive integer (whole rupiah)")
		return
	}

	cfg, err := s.resolveClientProvider(in.ProviderID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	svc, err := s.manager.Service(cfg.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "provider is not ready: "+err.Error())
		return
	}

	meta := make(map[string]any, len(in.Metadata)+1)
	for k, v := range in.Metadata {
		meta[k] = v
	}
	meta["_config_id"] = cfg.ID

	created, err := svc.CreatePayment(r.Context(), payment.CreatePaymentInput{
		Amount:    in.Amount,
		Reference: strings.TrimSpace(in.Reference),
		ExpiresIn: time.Duration(in.ExpiresInSeconds) * time.Second,
		Metadata:  meta,
	})
	if err != nil {
		switch {
		case core.IsErrorCode(err, core.CodeAmountPoolExhausted):
			writeError(w, http.StatusConflict, err.Error())
		case core.IsErrorCode(err, core.CodeAuthRequired), core.IsErrorCode(err, core.CodeAuthFailed):
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	row, err := s.repo.GetPaymentRow(created.ID)
	if err != nil || row == nil {
		writeError(w, http.StatusInternalServerError, "payment created but could not be loaded")
		return
	}
	writeJSON(w, http.StatusCreated, toClientPaymentDTO(*row))
}

func (s *Server) handleClientGetPayment(w http.ResponseWriter, r *http.Request) {
	row, err := s.repo.GetPaymentRow(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}
	writeJSON(w, http.StatusOK, toClientPaymentDTO(*row))
}

func (s *Server) handleClientCancelPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := s.repo.GetPaymentRow(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "payment not found")
		return
	}

	svc, err := s.manager.Service(row.ConfigID)
	if err != nil {
		writeError(w, http.StatusConflict, "provider is not ready: "+err.Error())
		return
	}
	if _, err := svc.CancelPayment(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.repo.GetPaymentRow(id)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "could not reload payment")
		return
	}
	writeJSON(w, http.StatusOK, toClientPaymentDTO(*updated))
}
