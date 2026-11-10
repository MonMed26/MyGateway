package webhook_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MonMed26/MyGateway/internal/db"
	"github.com/MonMed26/MyGateway/internal/model"
	"github.com/MonMed26/MyGateway/internal/repo"
	"github.com/MonMed26/MyGateway/internal/security"
	"github.com/MonMed26/MyGateway/internal/webhook"
	"github.com/MonMed26/MyGateway/utils"
)

func TestDispatcherDeliversSignedPayload(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store := repo.New(conn)
	if err := store.EnsureSettings("api-token", "secret"); err != nil {
		t.Fatalf("ensure settings: %v", err)
	}

	payload := []byte(`{"event":"payment.paid","payment":{"id":"pay1"}}`)
	var mu sync.Mutex
	var gotSignature string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotSignature = r.Header.Get("X-Gateway-Signature")
		gotBody = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	settings, err := store.GetSettings()
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	settings.WebhookURL = srv.URL
	settings.WebhookSecret = "secret"
	if err := store.SaveSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	d := webhook.New(store, utils.NoopLogger)
	d.Start()
	defer d.Stop()

	del, err := d.Enqueue("cfg1", "pay1", payload)
	if err != nil || del == nil {
		t.Fatalf("enqueue: %v %v", err, del)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := store.GetDelivery(del.ID)
		if err != nil {
			t.Fatalf("get delivery: %v", err)
		}
		if cur != nil && cur.Status == model.DeliverySuccess {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cur, _ := store.GetDelivery(del.ID)
	if cur == nil || cur.Status != model.DeliverySuccess || cur.Attempts != 1 {
		t.Fatalf("delivery not successful: %+v", cur)
	}

	mu.Lock()
	sig, body := gotSignature, gotBody
	mu.Unlock()
	want := "sha256=" + security.SignWebhook("secret", payload)
	if sig != want {
		t.Fatalf("signature mismatch: got %q want %q", sig, want)
	}
	if string(body) != string(payload) {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestDispatcherSkipsDisabledWebhook(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	store := repo.New(conn)
	if err := store.EnsureSettings("api-token", "secret"); err != nil {
		t.Fatalf("ensure settings: %v", err)
	}
	d := webhook.New(store, utils.NoopLogger)

	del, err := d.Enqueue("cfg1", "pay1", []byte("{}"))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if del != nil {
		t.Fatal("expected nil delivery when webhook is not configured")
	}
}
