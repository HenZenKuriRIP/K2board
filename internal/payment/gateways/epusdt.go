package gateways

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// EpusdtGateway integrates GMWalletApp/epusdt (GM Pay) multi-chain crypto gateway.
// Docs: https://github.com/GMWalletApp/epusdt/blob/master/wiki/API.md
// Create: POST {base}/payments/gmpay/v1/order/create-transaction
type EpusdtGateway struct {
	HTTPClient *http.Client
}

func (EpusdtGateway) Code() string { return "epusdt" }
func (EpusdtGateway) Name() string { return "USDT (Epusdt/GMPay)" }

type epusdtConfig struct {
	BaseURL   string `json:"base_url"`
	PID       string `json:"pid"`        // merchant PID, default 1000
	SecretKey string `json:"secret_key"` // API secret for MD5 sign
	// Legacy alias accepted for convenience
	APIToken string `json:"api_token"`

	// Token/Network: both empty (or "auto") → omit on create (GMPay status=4 cashier select).
	// Both set → lock parent order to that chain. Must not set only one.
	Token   string `json:"token"`
	Network string `json:"network"`
	// CashierSelect forces status-4 create even if token/network are set in config.
	// Prefer empty token+network instead; this flag is for explicit override.
	CashierSelect *bool `json:"cashier_select"`
	// Currency fiat for GMPay (cny/usd). Empty → from order.Currency lowercased.
	Currency string `json:"currency"`
	// RewritePaymentHost replaces internal IP in payment_url with base_url host (recommended).
	RewritePaymentHost *bool `json:"rewrite_payment_host"`
}

func parseEpusdtConfig(cfg string) (epusdtConfig, error) {
	var c epusdtConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("invalid epusdt config JSON: %w", err)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.PID = strings.TrimSpace(c.PID)
	if c.PID == "" {
		c.PID = "1000"
	}
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	if c.SecretKey == "" {
		c.SecretKey = strings.TrimSpace(c.APIToken)
	}
	c.Token = normalizeAssetField(c.Token)
	c.Network = normalizeAssetField(c.Network)
	c.Currency = strings.ToLower(strings.TrimSpace(c.Currency))
	return c, nil
}

// normalizeAssetField maps empty / "auto" / "any" to "" (cashier select).
func normalizeAssetField(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "auto", "any", "cashier", "select":
		return ""
	default:
		return s
	}
}

// useCashierSelect returns true when create should omit token+network (status 4).
func (c epusdtConfig) useCashierSelect() bool {
	if c.CashierSelect != nil {
		return *c.CashierSelect
	}
	// Default: both empty → cashier select; both set → lock chain.
	return c.Token == "" && c.Network == ""
}

func (g EpusdtGateway) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func (g EpusdtGateway) CreatePayment(ctx context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	cfg, err := parseEpusdtConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("epusdt: base_url 未配置")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("epusdt: secret_key 未配置（管理后台 API Keys 中的密钥）")
	}
	if opts.NotifyURL == "" {
		return nil, fmt.Errorf("epusdt: 缺少 notify_url（请配置面板 site_url）")
	}

	currency := cfg.Currency
	if currency == "" {
		currency = strings.ToLower(order.Currency)
	}
	if currency == "" {
		currency = "cny"
	}

	// order_id max 32 chars per GMPay docs
	orderID := order.TradeNo
	if len(orderID) > 32 {
		orderID = orderID[:32]
	}

	amountStr := payment.FormatFiatFromCents(order.TotalAmount)
	// GMPay: JSON numbers may normalize 1.00 → 1 in signature; use string form that
	// matches how we send the value. Prefer fixed 2 decimals for amount>=0.01.
	// Doc notes 100.00 may become 100 — use integer string when cents%100==0.
	signAmount := amountStr
	if order.TotalAmount%100 == 0 {
		signAmount = strconv.FormatInt(order.TotalAmount/100, 10)
	}

	// GMPay: token+network must both be present or both omitted.
	// Omitting creates status=4 placeholder; first /pay/switch-network fills the
	// parent in-place (no child). Locking to tron then switching to solana creates
	// a child order — dual rows in upay admin are by design in that mode.
	cashierSelect := cfg.useCashierSelect()
	token, network := cfg.Token, cfg.Network
	if cashierSelect {
		token, network = "", ""
	} else if (token == "") != (network == "") {
		return nil, fmt.Errorf("epusdt: token 与 network 必须同时填写或同时留空（留空=收银台自选链）")
	}

	signParams := map[string]string{
		"pid":          cfg.PID,
		"order_id":     orderID,
		"currency":     currency,
		"amount":       signAmount,
		"notify_url":   opts.NotifyURL,
		"redirect_url": opts.RedirectURL,
		"name":         order.PlanName,
	}
	if token != "" {
		signParams["token"] = token
		signParams["network"] = network
	}
	// redirect_url optional — omit empty from sign
	if opts.RedirectURL == "" {
		delete(signParams, "redirect_url")
	}
	if order.PlanName == "" {
		delete(signParams, "name")
		signParams["name"] = "K2Board Plan"
	}
	sig := payment.SignMD5(signParams, cfg.SecretKey)

	amountNum, _ := strconv.ParseFloat(amountStr, 64)
	body := map[string]any{
		"pid":        cfg.PID,
		"order_id":   orderID,
		"currency":   currency,
		"amount":     amountNum,
		"notify_url": opts.NotifyURL,
		"name":       signParams["name"],
		"signature":  sig,
	}
	if token != "" {
		body["token"] = token
		body["network"] = network
	}
	if opts.RedirectURL != "" {
		body["redirect_url"] = opts.RedirectURL
	}
	raw, _ := json.Marshal(body)

	endpoint := cfg.BaseURL + "/payments/gmpay/v1/order/create-transaction"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("epusdt request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID         string  `json:"trade_id"`
			OrderID         string  `json:"order_id"`
			Amount          any     `json:"amount"`
			Currency        string  `json:"currency"`
			ActualAmount    any     `json:"actual_amount"`
			ReceiveAddress  string  `json:"receive_address"`
			Token           string  `json:"token"`
			Status          any     `json:"status"`
			ExpirationTime  int64   `json:"expiration_time"`
			PaymentURL      string  `json:"payment_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("epusdt invalid response: %s", truncate(string(respBody), 240))
	}
	if result.StatusCode != 200 {
		msg := result.Message
		if msg == "" {
			msg = string(respBody)
		}
		return nil, fmt.Errorf("epusdt error: %s", msg)
	}
	if result.Data.PaymentURL == "" {
		return nil, fmt.Errorf("epusdt: empty payment_url")
	}

	payURL := result.Data.PaymentURL
	rewrite := true
	if cfg.RewritePaymentHost != nil {
		rewrite = *cfg.RewritePaymentHost
	}
	if rewrite {
		payURL = rewriteHost(payURL, cfg.BaseURL)
	}

	expireAt := result.Data.ExpirationTime
	// absolute unix if large; else treat as duration seconds
	if expireAt > 0 && expireAt < 1e9 {
		expireAt = time.Now().Unix() + expireAt
	}
	if expireAt <= 0 {
		expireAt = order.ExpiredAt.Unix()
	}

	actual := fmt.Sprint(result.Data.ActualAmount)
	msg := "请打开收银台完成 USDT 支付"
	if cashierSelect {
		msg = "请在收银台选择网络后完成支付（首次选择会绑定父订单，无需切链则不会产生子订单）"
	} else if token != "" {
		msg = fmt.Sprintf("请使用 %s/%s 支付约 %s", strings.ToUpper(token), network, actual)
	} else if result.Data.Token != "" {
		msg = fmt.Sprintf("请使用 %s 支付约 %s", result.Data.Token, actual)
	}
	return &payment.PaymentIntent{
		Type:       payment.IntentRedirect,
		URL:        payURL,
		PayAddress: result.Data.ReceiveAddress,
		TradeNo:    order.TradeNo,
		Amount:     order.TotalAmount,
		Currency:   order.Currency,
		Message:    msg,
		ExpireAt:   expireAt,
		Extra: map[string]any{
			"gateway":         "epusdt",
			"trade_id":        result.Data.TradeID,
			"payment_url":     payURL,
			"receive_address": result.Data.ReceiveAddress,
			"actual_amount":   result.Data.ActualAmount,
			"token":           result.Data.Token,
			"network":         network, // empty when status-4 placeholder
			"order_id_sent":   orderID,
			"cashier_select":  cashierSelect,
		},
	}, nil
}

func rewriteHost(paymentURL, baseURL string) string {
	pu, err1 := url.Parse(paymentURL)
	bu, err2 := url.Parse(baseURL)
	if err1 != nil || err2 != nil || pu == nil || bu == nil || bu.Host == "" {
		return paymentURL
	}
	// Always prefer public base host (epusdt often returns internal http://ip:8000)
	pu.Scheme = bu.Scheme
	if pu.Scheme == "" {
		pu.Scheme = "https"
	}
	pu.Host = bu.Host
	return pu.String()
}

// gmpayNotify is async callback body for GMPay orders.
type gmpayNotify struct {
	PID                string `json:"pid"`
	TradeID            string `json:"trade_id"`
	OrderID            string `json:"order_id"`
	Amount             any    `json:"amount"`
	ActualAmount       any    `json:"actual_amount"`
	ReceiveAddress     string `json:"receive_address"`
	Token              string `json:"token"`
	BlockTransactionID string `json:"block_transaction_id"`
	Signature          string `json:"signature"`
	Status             any    `json:"status"`
}

func (g EpusdtGateway) HandleNotify(ctx context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	_ = ctx
	cfg, err := parseEpusdtConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("epusdt: secret_key missing")
	}

	var n gmpayNotify
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("epusdt: invalid notify body")
	}

	status := anyToInt(n.Status)
	signParams := map[string]string{
		"pid":                  n.PID,
		"trade_id":             n.TradeID,
		"order_id":             n.OrderID,
		"amount":               anyToAmountString(n.Amount),
		"actual_amount":        anyToAmountString(n.ActualAmount),
		"receive_address":      n.ReceiveAddress,
		"token":                n.Token,
		"block_transaction_id": n.BlockTransactionID,
		"status":               strconv.Itoa(status),
	}
	expect := payment.SignMD5(signParams, cfg.SecretKey)
	if !strings.EqualFold(expect, strings.TrimSpace(n.Signature)) {
		// Some deployments stringify amounts differently — try 2-decimal fiat form
		if f := anyToFloat(n.Amount); f > 0 {
			signParams["amount"] = payment.FormatFiatFromCents(payment.FiatToCents(f))
			if alt := payment.SignMD5(signParams, cfg.SecretKey); strings.EqualFold(alt, strings.TrimSpace(n.Signature)) {
				goto verified
			}
		}
		return nil, fmt.Errorf("epusdt: invalid signature")
	}
verified:

	if status != 2 {
		return &payment.NotifyResult{
			TradeNo: n.OrderID,
			Success: false,
			Raw:     string(body),
		}, nil
	}

	return &payment.NotifyResult{
		TradeNo:    n.OrderID,
		PaidAmount: payment.FiatToCents(anyToFloat(n.Amount)),
		Success:    true,
		CallbackNo: firstNonEmpty(n.BlockTransactionID, n.TradeID),
		Raw:        string(body),
	}, nil
}

func (g EpusdtGateway) QueryPayment(ctx context.Context, order *models.Order, cfgJSON string) (*payment.QueryResult, error) {
	cfg, err := parseEpusdtConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	// Prefer trade_id from order meta if present
	tradeID := ""
	if order.Meta != "" {
		var m map[string]any
		if json.Unmarshal([]byte(order.Meta), &m) == nil {
			if v, ok := m["trade_id"].(string); ok {
				tradeID = v
			}
		}
	}
	if tradeID == "" || cfg.BaseURL == "" {
		return &payment.QueryResult{
			Paid:       order.Status == models.OrderPaid,
			PaidAmount: order.TotalAmount,
			CallbackNo: order.CallbackNo,
		}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/pay/check-status/"+url.PathEscape(tradeID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			Status any `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &result)
	st := anyToInt(result.Data.Status)
	return &payment.QueryResult{
		Paid:       st == 2,
		PaidAmount: order.TotalAmount,
		CallbackNo: tradeID,
	}, nil
}

func init() {
	payment.Register(EpusdtGateway{})
}
