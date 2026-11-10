// Package httpapi exposes the admin dashboard API and the client payment API.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hirotomasato/paygateme/internal/gateway"
	"github.com/hirotomasato/paygateme/internal/repo"
	"github.com/hirotomasato/paygateme/internal/security"
	"github.com/hirotomasato/paygateme/internal/webhook"
	"github.com/hirotomasato/paygateme/utils"
)

// Server wires the HTTP layer to the repositories and gateway manager.
type Server struct {
	repo       *repo.Store
	manager    *gateway.Manager
	dispatcher *webhook.Dispatcher
	logger     utils.Logger
	ui         http.Handler
	cookieName string
}

// New constructs an API server. ui may be nil (API-only mode).
func New(r *repo.Store, manager *gateway.Manager, dispatcher *webhook.Dispatcher, logger utils.Logger, ui http.Handler) *Server {
	if logger == nil {
		logger = utils.NoopLogger
	}
	return &Server{
		repo:       r,
		manager:    manager,
		dispatcher: dispatcher,
		logger:     logger,
		ui:         ui,
		cookieName: "gw_admin",
	}
}

type ctxKey int

const ctxUserID ctxKey = iota

func userIDFrom(r *http.Request) int64 {
	v, _ := r.Context().Value(ctxUserID).(int64)
	return v
}

// Handler builds the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// Admin auth.
	mux.HandleFunc("POST /api/admin/login", s.handleLogin)
	mux.HandleFunc("POST /api/admin/logout", s.admin(s.handleLogout))
	mux.HandleFunc("GET /api/admin/me", s.admin(s.handleMe))

	// Dashboard.
	mux.HandleFunc("GET /api/admin/stats", s.admin(s.handleStats))

	// Global settings.
	mux.HandleFunc("GET /api/admin/settings", s.admin(s.handleGetSettings))
	mux.HandleFunc("PATCH /api/admin/settings", s.admin(s.handleUpdateSettings))
	mux.HandleFunc("POST /api/admin/settings/rotate-api-token", s.admin(s.handleRotateAPIToken))

	// Providers.
	mux.HandleFunc("GET /api/admin/providers", s.admin(s.handleListProviders))
	mux.HandleFunc("POST /api/admin/providers", s.admin(s.handleCreateProvider))
	mux.HandleFunc("GET /api/admin/providers/{id}", s.admin(s.handleGetProvider))
	mux.HandleFunc("PATCH /api/admin/providers/{id}", s.admin(s.handleUpdateProvider))
	mux.HandleFunc("DELETE /api/admin/providers/{id}", s.admin(s.handleDeleteProvider))
	mux.HandleFunc("POST /api/admin/providers/{id}/otp/request", s.admin(s.handleRequestOtp))
	mux.HandleFunc("POST /api/admin/providers/{id}/otp/verify", s.admin(s.handleVerifyOtp))
	mux.HandleFunc("POST /api/admin/providers/{id}/complete-login", s.admin(s.handleCompleteLogin))
	mux.HandleFunc("POST /api/admin/providers/{id}/select-merchant", s.admin(s.handleSelectMerchant))
	mux.HandleFunc("POST /api/admin/providers/{id}/select-store", s.admin(s.handleSelectStore))
	mux.HandleFunc("POST /api/admin/providers/{id}/select-outlet", s.admin(s.handleSelectOutlet))
	mux.HandleFunc("POST /api/admin/providers/{id}/refresh", s.admin(s.handleRefresh))
	mux.HandleFunc("POST /api/admin/providers/{id}/qris", s.admin(s.handleSetQRIS))
	mux.HandleFunc("GET /api/admin/providers/{id}/status", s.admin(s.handleProviderStatus))
	mux.HandleFunc("GET /api/admin/providers/{id}/stores", s.admin(s.handleListStores))
	mux.HandleFunc("GET /api/admin/providers/{id}/merchants", s.admin(s.handleListMerchants))

	// Transactions.
	mux.HandleFunc("GET /api/admin/transactions", s.admin(s.handleListTransactions))
	mux.HandleFunc("GET /api/admin/transactions/{id}", s.admin(s.handleGetTransaction))
	mux.HandleFunc("GET /api/admin/transactions/{id}/webhooks", s.admin(s.handleListWebhooks))
	mux.HandleFunc("POST /api/admin/transactions/{id}/resend-webhook", s.admin(s.handleResendWebhook))

	// Client API v1 (Bearer token).
	mux.HandleFunc("GET /api/v1/ping", s.client(s.handlePing))
	mux.HandleFunc("POST /api/v1/payments", s.client(s.handleClientCreatePayment))
	mux.HandleFunc("GET /api/v1/payments/{id}", s.client(s.handleClientGetPayment))
	mux.HandleFunc("POST /api/v1/payments/{id}/cancel", s.client(s.handleClientCancelPayment))

	// Static dashboard (SPA).
	if s.ui != nil {
		mux.Handle("/", s.ui)
	}

	return s.recover(s.logRequests(mux))
}

// ---- Middleware ----

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		uid, ok, err := s.repo.AdminSessionValid(security.HashToken(c.Value))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "session lookup failed")
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) client(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		settings, err := s.repo.GetSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token lookup failed")
			return
		}
		if settings.APIToken == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(settings.APIToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next(w, r)
	}
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("http: panic recovered", map[string]any{"path": r.URL.Path})
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			start := time.Now()
			next.ServeHTTP(w, r)
			s.logger.Debug("http request", map[string]any{
				"method": r.Method, "path": r.URL.Path, "duration_ms": time.Since(start).Milliseconds(),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httpapi: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
