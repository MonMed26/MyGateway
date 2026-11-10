package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/hirotomasato/paygateme/core"
	"github.com/hirotomasato/paygateme/internal/model"
	"github.com/hirotomasato/paygateme/internal/security"
	"github.com/hirotomasato/paygateme/qris"
	"github.com/hirotomasato/paygateme/shopee"
)

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	configs, err := s.repo.ListProviderConfigs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load providers")
		return
	}
	items := make([]providerDTO, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, toProviderDTO(cfg))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if in.Provider != model.ProviderShopee && in.Provider != model.ProviderGoPay {
		writeError(w, http.StatusBadRequest, "provider must be shopee or gopay")
		return
	}
	now := time.Now()
	cfg := model.ProviderConfig{
		ID:             security.NewToken(8),
		Name:           in.Name,
		Provider:       in.Provider,
		PollIntervalMS: 3000,
		ExpiryMS:       300000,
		ClockSkewMS:    60000,
		Active:         true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateProviderConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create provider")
		return
	}
	_ = s.manager.Rebuild(cfg.ID)
	writeJSON(w, http.StatusCreated, toProviderDTO(cfg))
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	writeJSON(w, http.StatusOK, toProviderDTO(*cfg))
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	var in struct {
		Name           *string `json:"name"`
		PollIntervalMS *int64  `json:"poll_interval_ms"`
		ExpiryMS       *int64  `json:"expiry_ms"`
		ClockSkewMS    *int64  `json:"clock_skew_ms"`
		StaticQRIS     *string `json:"static_qris"`
		Active         *bool   `json:"active"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		cfg.Name = name
	}
	if in.PollIntervalMS != nil {
		cfg.PollIntervalMS = *in.PollIntervalMS
	}
	if in.ExpiryMS != nil {
		cfg.ExpiryMS = *in.ExpiryMS
	}
	if in.ClockSkewMS != nil {
		cfg.ClockSkewMS = *in.ClockSkewMS
	}
	if in.Active != nil {
		cfg.Active = *in.Active
	}
	if in.StaticQRIS != nil {
		payload := strings.TrimSpace(*in.StaticQRIS)
		if payload != "" && !qris.IsValidQRISChecksum(payload) {
			writeError(w, http.StatusBadRequest, "static QRIS checksum is invalid")
			return
		}
		cfg.StaticQRIS = payload
	}
	if cfg.PollIntervalMS < 500 {
		cfg.PollIntervalMS = 3000
	}
	if cfg.ExpiryMS < 15000 {
		cfg.ExpiryMS = 300000
	}
	if cfg.ClockSkewMS < 0 {
		cfg.ClockSkewMS = 60000
	}

	if err := s.repo.UpdateProviderConfig(*cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update provider")
		return
	}
	_ = s.manager.Rebuild(cfg.ID)

	updated, _ := s.repo.GetProviderConfig(cfg.ID)
	if updated == nil {
		updated = cfg
	}
	writeJSON(w, http.StatusOK, toProviderDTO(*updated))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.repo.DeleteProviderConfig(id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete provider")
		return
	}
	_ = s.manager.Rebuild(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRequestOtp(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var in struct {
		Phone        string `json:"phone"`
		Password     string `json:"password"`
		DeviceReport string `json:"device_report"`
		CountryCode  string `json:"country_code"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	phone := strings.TrimSpace(in.Phone)
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone is required")
		return
	}

	switch cfg.Provider {
	case model.ProviderShopee:
		ch, err := s.manager.RequestShopeeOtp(r.Context(), cfg.ID, phone, in.Password, in.DeviceReport)
		if err != nil {
			writeProviderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":           model.ProviderShopee,
			"phone":              ch.PhoneNumber,
			"channel":            ch.Channel,
			"has_password":       ch.HasPassword,
			"available_channels": ch.AvailableChannels,
		})
	case model.ProviderGoPay:
		res, err := s.manager.RequestGoPayOtp(r.Context(), cfg.ID, phone, in.CountryCode)
		if err != nil {
			writeProviderError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":  model.ProviderGoPay,
			"otp_token": res.OTPToken,
		})
	default:
		writeError(w, http.StatusBadRequest, "unsupported provider")
	}
}

func (s *Server) handleVerifyOtp(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var in struct {
		OTP        string `json:"otp"`
		OTPToken   string `json:"otp_token"`
		MerchantID string `json:"merchant_id"`
		StoreID    string `json:"store_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.OTP = strings.TrimSpace(in.OTP)
	if in.OTP == "" {
		writeError(w, http.StatusBadRequest, "otp is required")
		return
	}

	switch cfg.Provider {
	case model.ProviderShopee:
		outcome, err := s.manager.VerifyShopeeOtp(r.Context(), cfg.ID, in.OTP)
		if err != nil {
			writeProviderError(w, err)
			return
		}
		if outcome.Status == shopee.LoginMerchantSelectionNeeded {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    "merchant_selection",
				"merchants": outcome.Merchants,
			})
			return
		}
		st, _ := s.manager.Status(cfg.ID)
		writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "provider": st})
	case model.ProviderGoPay:
		if in.OTPToken == "" {
			writeError(w, http.StatusBadRequest, "otp_token is required for GoPay")
			return
		}
		if _, err := s.manager.VerifyGoPayOtp(r.Context(), cfg.ID, in.OTP, in.OTPToken); err != nil {
			writeProviderError(w, err)
			return
		}
		st, _ := s.manager.Status(cfg.ID)
		writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "provider": st})
	default:
		writeError(w, http.StatusBadRequest, "unsupported provider")
	}
}

func (s *Server) handleCompleteLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if cfg.Provider != model.ProviderShopee {
		writeError(w, http.StatusBadRequest, "complete-login is only valid for Shopee")
		return
	}
	var in struct {
		MerchantID string `json:"merchant_id"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.MerchantID) == "" {
		writeError(w, http.StatusBadRequest, "merchant_id is required")
		return
	}
	if _, err := s.manager.CompleteShopeeLogin(r.Context(), cfg.ID, in.MerchantID); err != nil {
		writeProviderError(w, err)
		return
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleSelectMerchant(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var in struct {
		MerchantID string `json:"merchant_id"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.MerchantID) == "" {
		writeError(w, http.StatusBadRequest, "merchant_id is required")
		return
	}
	switch cfg.Provider {
	case model.ProviderShopee:
		if _, err := s.manager.SelectShopeeMerchant(r.Context(), cfg.ID, in.MerchantID); err != nil {
			writeProviderError(w, err)
			return
		}
	case model.ProviderGoPay:
		if _, err := s.manager.SelectGoPayMerchant(r.Context(), cfg.ID, in.MerchantID); err != nil {
			writeProviderError(w, err)
			return
		}
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleSelectStore(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var in struct {
		StoreID  string `json:"store_id"`
		OutletID string `json:"outlet_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target := in.StoreID
	if target == "" {
		target = in.OutletID
	}
	if strings.TrimSpace(target) == "" {
		writeError(w, http.StatusBadRequest, "store_id is required")
		return
	}
	switch cfg.Provider {
	case model.ProviderShopee:
		if _, err := s.manager.SelectShopeeStore(r.Context(), cfg.ID, target); err != nil {
			writeProviderError(w, err)
			return
		}
	case model.ProviderGoPay:
		if _, err := s.manager.SelectGoPayOutlet(r.Context(), cfg.ID, target); err != nil {
			writeProviderError(w, err)
			return
		}
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleSelectOutlet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if cfg.Provider != model.ProviderGoPay {
		writeError(w, http.StatusBadRequest, "select-outlet is only valid for GoPay")
		return
	}
	var in struct {
		PopID    string `json:"pop_id"`
		OutletID string `json:"outlet_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	popID := in.PopID
	if popID == "" {
		popID = in.OutletID
	}
	if popID == "" {
		writeError(w, http.StatusBadRequest, "pop_id is required")
		return
	}
	if _, err := s.manager.SelectGoPayOutlet(r.Context(), cfg.ID, popID); err != nil {
		writeProviderError(w, err)
		return
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if err := s.manager.Refresh(r.Context(), cfg.ID); err != nil {
		writeProviderError(w, err)
		return
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleSetQRIS(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	var in struct {
		Payload string `json:"payload"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.Payload) == "" {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}
	if err := s.manager.SetQRIS(r.Context(), cfg.ID, strings.TrimSpace(in.Payload)); err != nil {
		writeProviderError(w, err)
		return
	}
	s.writeStatus(w, cfg.ID)
}

func (s *Server) handleProviderStatus(w http.ResponseWriter, r *http.Request) {
	s.writeStatus(w, r.PathValue("id"))
}

func (s *Server) writeStatus(w http.ResponseWriter, id string) {
	st, err := s.manager.Status(id)
	if err != nil {
		writeProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleListStores(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if cfg.Provider != model.ProviderShopee {
		writeError(w, http.StatusBadRequest, "stores are only available for Shopee")
		return
	}
	stores, err := s.manager.ShopeeStores(r.Context(), cfg.ID)
	if err != nil {
		writeProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": stores})
}

func (s *Server) handleListMerchants(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.repo.GetProviderConfig(r.PathValue("id"))
	if err != nil || cfg == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if cfg.Provider != model.ProviderGoPay {
		writeError(w, http.StatusBadRequest, "merchants are only available for GoPay")
		return
	}
	merchants, err := s.manager.GoPayMerchants(r.Context(), cfg.ID)
	if err != nil {
		writeProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": merchants})
}

func writeProviderError(w http.ResponseWriter, err error) {
	switch {
	case core.IsErrorCode(err, core.CodeAuthRequired), core.IsErrorCode(err, core.CodeAuthFailed):
		writeError(w, http.StatusUnauthorized, err.Error())
	case core.IsErrorCode(err, core.CodeHTTPError):
		writeError(w, http.StatusBadGateway, err.Error())
	case core.IsErrorCode(err, core.CodeCaptchaRequired):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
