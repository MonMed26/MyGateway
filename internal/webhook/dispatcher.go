// Package webhook delivers signed payment events to merchant endpoints with
// exponential-backoff retries and manual resend support.
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/hirotomasato/paygateme/internal/model"
	"github.com/hirotomasato/paygateme/internal/repo"
	"github.com/hirotomasato/paygateme/internal/security"
	"github.com/hirotomasato/paygateme/utils"
)

// Dispatcher owns the outbound webhook queue.
type Dispatcher struct {
	repo        *repo.Store
	logger      utils.Logger
	client      *http.Client
	maxAttempts int

	wake chan struct{}
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// New creates a dispatcher.
func New(r *repo.Store, logger utils.Logger) *Dispatcher {
	if logger == nil {
		logger = utils.NoopLogger
	}
	return &Dispatcher{
		repo:        r,
		logger:      logger,
		client:      &http.Client{Timeout: 15 * time.Second},
		maxAttempts: 6,
		wake:        make(chan struct{}, 1),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Start launches the background delivery loop.
func (d *Dispatcher) Start() { go d.run() }

// Stop terminates the loop and waits for it to exit.
func (d *Dispatcher) Stop() {
	d.once.Do(func() { close(d.stop) })
	<-d.done
}

func (d *Dispatcher) notify() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) run() {
	defer close(d.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-d.wake:
		case <-ticker.C:
		}
		d.drain()
	}
}

func (d *Dispatcher) drain() {
	due, err := d.repo.DueDeliveries(time.Now(), 20)
	if err != nil {
		d.logger.Error("webhook: list due deliveries failed", map[string]any{"error": err.Error()})
		return
	}
	for _, del := range due {
		d.deliver(del)
	}
}

var eventHeaders = map[string]string{
	"Content-Type":      "application/json",
	"User-Agent":        "paygateme-gateway/1.0",
	"X-Gateway-Event":   "payment.paid",
	"X-Gateway-Version": "1",
}

func (d *Dispatcher) deliver(del model.WebhookDelivery) {
	attempts := del.Attempts + 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, del.URL, bytes.NewReader(del.Payload))
	if err != nil {
		d.finish(&del, attempts, 0, "invalid request: "+err.Error())
		return
	}
	for k, v := range eventHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-Gateway-Signature", "sha256="+del.Signature)
	req.Header.Set("X-Gateway-Delivery", del.ID)

	resp, err := d.client.Do(req)
	if err != nil {
		d.finish(&del, attempts, 0, "transport error: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	d.finish(&del, attempts, resp.StatusCode, string(body))
}

func (d *Dispatcher) finish(del *model.WebhookDelivery, attempts, statusCode int, response string) {
	del.Attempts = attempts
	del.LastStatusCode = statusCode
	del.LastResponse = truncate(response, 2000)

	success := statusCode >= 200 && statusCode < 300
	switch {
	case success:
		del.Status = model.DeliverySuccess
		del.NextAttemptAt = time.Time{}
		d.logger.Info("webhook: delivered", map[string]any{"delivery": del.ID, "status": statusCode})
	case attempts >= d.maxAttempts:
		del.Status = model.DeliveryFailed
		del.NextAttemptAt = time.Time{}
		d.logger.Error("webhook: giving up after max attempts", map[string]any{"delivery": del.ID, "attempts": attempts})
	default:
		del.Status = model.DeliveryPending
		del.NextAttemptAt = time.Now().Add(backoff(attempts))
		d.logger.Warn("webhook: retry scheduled", map[string]any{
			"delivery": del.ID, "attempts": attempts, "status": statusCode,
			"nextIn": time.Until(del.NextAttemptAt).String(),
		})
	}
	if err := d.repo.UpdateDelivery(*del); err != nil {
		d.logger.Error("webhook: persist delivery failed", map[string]any{"error": err.Error()})
	}
}

func backoff(attempt int) time.Duration {
	d := 15 * time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > time.Hour {
			return time.Hour
		}
	}
	return d
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Enqueue creates a delivery for a paid payment using the global webhook
// settings. It returns nil when no webhook URL/secret is configured.
func (d *Dispatcher) Enqueue(configID, paymentID string, payload []byte) (*model.WebhookDelivery, error) {
	settings, err := d.repo.GetSettings()
	if err != nil {
		return nil, err
	}
	if settings.WebhookURL == "" || settings.WebhookSecret == "" {
		return nil, nil
	}
	now := time.Now()
	del := model.WebhookDelivery{
		ID:            security.NewToken(16),
		ConfigID:      configID,
		PaymentID:     paymentID,
		URL:           settings.WebhookURL,
		Payload:       payload,
		Signature:     security.SignWebhook(settings.WebhookSecret, payload),
		Status:        model.DeliveryPending,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := d.repo.CreateDelivery(del); err != nil {
		return nil, err
	}
	d.notify()
	return &del, nil
}

// Resend queues a fresh delivery cloned from an existing one.
func (d *Dispatcher) Resend(deliveryID string) (*model.WebhookDelivery, error) {
	prev, err := d.repo.GetDelivery(deliveryID)
	if err != nil {
		return nil, err
	}
	if prev == nil {
		return nil, fmt.Errorf("delivery not found")
	}
	now := time.Now()
	del := model.WebhookDelivery{
		ID:            security.NewToken(16),
		ConfigID:      prev.ConfigID,
		PaymentID:     prev.PaymentID,
		URL:           prev.URL,
		Payload:       prev.Payload,
		Signature:     prev.Signature,
		Status:        model.DeliveryPending,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := d.repo.CreateDelivery(del); err != nil {
		return nil, err
	}
	d.notify()
	return &del, nil
}
