package payment

import (
	"context"
	"strings"
	"sync"

	"K2board/internal/models"
)

// IntentType describes how the client should complete payment.
const (
	IntentRedirect  = "redirect"
	IntentQRCode    = "qrcode"
	IntentAddress   = "address"
	IntentMock      = "mock"
	IntentCompleted = "completed" // already paid (e.g. free / mock instant)
)

// PaymentIntent is returned by CreatePayment for the checkout UI.
type PaymentIntent struct {
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	QRContent   string `json:"qr_content,omitempty"`
	PayAddress  string `json:"pay_address,omitempty"`
	Message     string `json:"message,omitempty"`
	TradeNo     string `json:"trade_no"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	ExpireAt    int64  `json:"expire_at,omitempty"` // unix
	Extra       any    `json:"extra,omitempty"`
}

// NotifyResult is produced by HandleNotify after signature verification.
type NotifyResult struct {
	TradeNo    string
	PaidAmount int64
	Success    bool
	CallbackNo string
	Raw        string
}

// QueryResult is produced by QueryPayment for reconciliation.
type QueryResult struct {
	Paid       bool
	PaidAmount int64
	CallbackNo string
}

// Gateway is a pluggable payment channel.
type Gateway interface {
	Code() string
	Name() string
	// CreatePayment starts a charge for the order. cfg is method-specific JSON.
	CreatePayment(ctx context.Context, order *models.Order, cfg string, opts CreateOptions) (*PaymentIntent, error)
	// HandleNotify processes async provider callbacks (optional for mock).
	HandleNotify(ctx context.Context, headers map[string]string, body []byte, cfg string) (*NotifyResult, error)
	// QueryPayment polls provider for payment status (optional).
	QueryPayment(ctx context.Context, order *models.Order, cfg string) (*QueryResult, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Gateway{}
)

// Register adds a gateway implementation.
func Register(g Gateway) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[g.Code()] = g
}

// Get returns a registered gateway by payment method code.
// Supports multi-instance codes: frog_alipay / frog_wx → gateway "frog".
func Get(code string) (Gateway, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, false
	}
	regMu.RLock()
	defer regMu.RUnlock()
	if g, ok := registry[code]; ok {
		return g, true
	}
	// Instance suffix: {gateway}_{label} e.g. frog_alipay, epay_wx
	if i := strings.IndexByte(code, '_'); i > 0 {
		base := code[:i]
		if g, ok := registry[base]; ok {
			return g, true
		}
	}
	return nil, false
}

// BaseCode returns the registered gateway id for a payment method code.
// frog_alipay → frog; epay → epay.
func BaseCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	regMu.RLock()
	defer regMu.RUnlock()
	if _, ok := registry[code]; ok {
		return code
	}
	if i := strings.IndexByte(code, '_'); i > 0 {
		base := code[:i]
		if _, ok := registry[base]; ok {
			return base
		}
	}
	return code
}

// MultiInstance reports whether multiple payment_methods rows may share one gateway
// via codes like frog_alipay, frog_wx (different channel config each).
func MultiInstance(gatewayCode string) bool {
	switch BaseCode(gatewayCode) {
	case "frog", "epay":
		return true
	default:
		return false
	}
}

// ListCodes returns registered gateway codes.
func ListCodes() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
