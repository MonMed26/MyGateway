package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/MonMed26/MyGateway/internal/gateway"
	"github.com/MonMed26/MyGateway/internal/repo"
)

func atoiDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func (s *Server) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	size := atoiDefault(q.Get("page_size"), 20)
	rows, total, err := s.repo.ListPayments(repo.PaymentFilter{
		ConfigID: q.Get("config_id"),
		Status:   q.Get("status"),
		Query:    q.Get("q"),
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load transactions")
		return
	}
	items := make([]paymentDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toPaymentDTO(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": size,
	})
}

func (s *Server) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := s.repo.GetPaymentRow(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	deliveries, _ := s.repo.DeliveriesByPayment(id)
	items := make([]deliveryDTO, 0, len(deliveries))
	for _, d := range deliveries {
		items = append(items, toDeliveryDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"payment":  toPaymentDTO(*row),
		"webhooks": items,
	})
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	deliveries, err := s.repo.DeliveriesByPayment(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	items := make([]deliveryDTO, 0, len(deliveries))
	for _, d := range deliveries {
		items = append(items, toDeliveryDTO(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleResendWebhook(w http.ResponseWriter, r *http.Request) {
	row, err := s.repo.GetPaymentRow(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	if strings.ToLower(row.Status) != "paid" {
		writeError(w, http.StatusBadRequest, "webhooks can only be resent for paid transactions")
		return
	}
	cfg, err := s.repo.GetProviderConfig(row.ConfigID)
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider config not found")
		return
	}
	settings, err := s.repo.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	if settings.WebhookURL == "" || settings.WebhookSecret == "" {
		writeError(w, http.StatusBadRequest, "webhook is not configured in Settings")
		return
	}
	payload := gateway.EventPayload(*cfg, row.Core())
	delivery, err := s.dispatcher.Enqueue(row.ConfigID, row.ID, payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not enqueue webhook")
		return
	}
	if delivery == nil {
		writeError(w, http.StatusBadRequest, "webhook is not configured in Settings")
		return
	}
	writeJSON(w, http.StatusOK, toDeliveryDTO(*delivery))
}
