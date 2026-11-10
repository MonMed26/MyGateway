package gopay

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hirotomasato/paygateme/core"
)

// TransactionFeed reads settled/authorized transactions from the merchant
// analytics API. This is the signal source the gateway polls to detect
// incoming GoPay QRIS payments.
type TransactionFeed struct {
	http *HTTPClient
}

// NewTransactionFeed creates a feed adapter.
func NewTransactionFeed(http *HTTPClient) *TransactionFeed {
	return &TransactionFeed{http: http}
}

// ListRecent implements core.TransactionFeed.
func (f *TransactionFeed) ListRecent(ctx context.Context, query core.TransactionFeedQuery) (core.TransactionFeedResult, error) {
	merchantID := query.Scope.MerchantID
	if merchantID == "" {
		merchantID = query.Scope.AccountID
	}
	statuses := PaidTransactionStatuses
	paymentTypes := DefaultPaymentTypes

	size := query.PageSize
	if size <= 0 {
		size = DefaultTransactionPageSize
	}
	if size > MaxTransactionPageSize {
		size = MaxTransactionPageSize
	}
	maxPages := query.MaxPages
	if maxPages <= 0 {
		maxPages = MaxTransactionPagesPerTick
	}
	if maxPages > MaxTransactionPagesPerTick {
		maxPages = MaxTransactionPagesPerTick
	}

	baseParams := map[string]string{
		"size":         fmt.Sprintf("%d", size),
		"start_time":   query.StartTime.UTC().Format(time.RFC3339),
		"end_time":     query.EndTime.UTC().Format(time.RFC3339),
		"merchant_ids": merchantID,
	}
	if len(statuses) > 0 {
		baseParams["statuses"] = join(statuses, ",")
	}
	if len(paymentTypes) > 0 {
		baseParams["payment_types"] = join(paymentTypes, ",")
	}

	// The merchant-analytics feed is offset-based (`from`). Walk the window page
	// by page so high-volume merchants do not silently lose transactions that
	// fall beyond the first page.
	transactions := make([]core.MerchantTransaction, 0, size)
	from := 0
	pagesFetched := 0
	truncated := false
	lastLen := -1
	for pagesFetched < maxPages {
		params := make(map[string]string, len(baseParams)+1)
		for k, v := range baseParams {
			params[k] = v
		}
		params["from"] = fmt.Sprintf("%d", from)

		var raw struct {
			From         int              `json:"from"`
			Size         int              `json:"size"`
			Total        int              `json:"total"`
			Transactions []rawTransaction `json:"transactions"`
		}
		if err := f.http.RequestJSON(ctx, HTTPRequest{
			Method: "GET",
			Path:   epTransactions,
			Query:  params,
		}, &raw); err != nil {
			return core.TransactionFeedResult{}, err
		}

		pagesFetched++
		lastLen = len(raw.Transactions)
		for _, t := range raw.Transactions {
			tx := normalizeTransaction(t)
			transactions = append(transactions, tx.ToCoreTransaction())
		}

		from += lastLen
		if lastLen < size {
			break
		}
		if raw.Total > 0 && from >= raw.Total {
			break
		}
		if pagesFetched == maxPages {
			truncated = true
		}
	}

	return core.TransactionFeedResult{
		Transactions: transactions,
		Truncated:    truncated,
		PagesFetched: pagesFetched,
	}, nil
}

// ---- Wire types ----

type rawTransaction struct {
	ID                string  `json:"id"`
	OrderID           string  `json:"order_id"`
	MerchantID        string  `json:"merchant_id"`
	TransactionStatus string  `json:"transaction_status"`
	PaymentType       string  `json:"payment_type"`
	GrossAmount       float64 `json:"gross_amount"`
	RealGrossAmount   float64 `json:"real_gross_amount"`
	Currency          string  `json:"currency"`
	TransactionTime   string  `json:"transaction_time"`
	SettlementTime    string  `json:"settlement_time"`
	TransactionSource string  `json:"transaction_source"`
}

// ---- Normalization ----

func normalizeTransaction(raw rawTransaction) GoPayTransaction {
	currency := raw.Currency
	if currency == "" {
		currency = "IDR"
	}
	tx := GoPayTransaction{
		ID:                raw.ID,
		OrderID:           raw.OrderID,
		MerchantID:        raw.MerchantID,
		Status:            raw.TransactionStatus,
		PaymentType:       raw.PaymentType,
		GrossAmount:       toWholeRupiah(raw.GrossAmount),
		RealGrossAmount:   toWholeRupiah(raw.RealGrossAmount),
		Currency:          currency,
		TransactionTime:   raw.TransactionTime,
		SettlementTime:    raw.SettlementTime,
		TransactionSource: raw.TransactionSource,
		Raw:               mustMarshal(raw),
	}
	return tx
}

// toWholeRupiah converts a feed amount from minor units to whole rupiah.
// The feed sends 300100 for Rp 3.001. Division is exact: a value that is
// not a whole rupiah stays fractional and fails to match a whole-rupiah
// payment intent, which is the safe outcome.
func toWholeRupiah(minorUnits float64) int64 {
	if minorUnits != minorUnits { // NaN
		return 0
	}
	return int64(minorUnits / TransactionAmountScale)
}

// ToCoreTransaction converts a GoPayTransaction to the provider-agnostic
// core.MerchantTransaction.
func (tx GoPayTransaction) ToCoreTransaction() core.MerchantTransaction {
	rawData, _ := json.Marshal(tx.Raw)
	return core.MerchantTransaction{
		ID:          tx.ID,
		OrderID:     tx.OrderID,
		Status:      tx.Status,
		PaymentType: tx.PaymentType,
		Amount:      tx.GrossAmount,
		RealAmount:  tx.RealGrossAmount,
		Currency:    tx.Currency,
		Time:        parseTimeOrZero(tx.TransactionTime),
		Raw:         rawData,
	}
}

func join(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func parseTimeOrZero(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
