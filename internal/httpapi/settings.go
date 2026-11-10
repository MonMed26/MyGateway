package httpapi

import (
	"net/http"
	"strings"

	"github.com/hirotomasato/paygateme/internal/security"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.repo.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	writeJSON(w, http.StatusOK, toSettingsDTO(st))
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.repo.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}

	var in struct {
		WebhookURL        *string `json:"webhook_url"`
		WebhookSecret     *string `json:"webhook_secret"`
		UniqueMin         *int64  `json:"unique_min"`
		UniqueMax         *int64  `json:"unique_max"`
		DefaultProviderID *string `json:"default_provider_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if in.WebhookURL != nil {
		st.WebhookURL = strings.TrimSpace(*in.WebhookURL)
	}
	if in.WebhookSecret != nil {
		st.WebhookSecret = strings.TrimSpace(*in.WebhookSecret)
	}
	if in.UniqueMin != nil {
		st.UniqueMin = *in.UniqueMin
	}
	if in.UniqueMax != nil {
		st.UniqueMax = *in.UniqueMax
	}
	if in.DefaultProviderID != nil {
		id := strings.TrimSpace(*in.DefaultProviderID)
		if id != "" {
			cfg, err := s.repo.GetProviderConfig(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "lookup provider failed")
				return
			}
			if cfg == nil {
				writeError(w, http.StatusBadRequest, "default provider not found")
				return
			}
		}
		st.DefaultProviderID = id
	}

	if st.UniqueMin < 0 {
		writeError(w, http.StatusBadRequest, "unique_min cannot be negative")
		return
	}
	if st.UniqueMax < 1 || st.UniqueMax < st.UniqueMin {
		writeError(w, http.StatusBadRequest, "unique_max must be >= unique_min and >= 1")
		return
	}

	if err := s.repo.SaveSettings(st); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	s.manager.ReloadSettings()
	s.manager.RebuildAll()
	writeJSON(w, http.StatusOK, toSettingsDTO(st))
}

func (s *Server) handleRotateAPIToken(w http.ResponseWriter, r *http.Request) {
	st, err := s.repo.GetSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	st.APIToken = security.NewToken(32)
	if err := s.repo.SaveSettings(st); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_token": st.APIToken})
}
