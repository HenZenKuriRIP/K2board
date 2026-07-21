package gateways

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// BEpusdtGateway integrates https://github.com/v03413/bepusdt as an external USDT cashier.
// Works offline for notify/signature unit paths; CreatePayment needs a reachable base_url.
type BEpusdtGateway struct {
	HTTPClient *http.Client
}

func (BEpusdtGateway) Code() string { return "bepusdt" }
func (BEpusdtGateway) Name() string { return "USDT (BEpusdt)" }

type bepusdtConfig struct {
	BaseURL   string `json:"base_url"`
	APIToken  string `json:"api_token"`
	TradeType string `json:"trade_type"`
	Timeout   int    `json:"timeout"` // seconds for BEpusdt order
	Fiat      string `json:"fiat"`    // CNY / USD …
}

func parseBepusdtConfig(cfg string) (bepusdtConfig, error) {
	var c bepusdtConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("invalid bepusdt config JSON: %w", err)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.APIToken = strings.TrimSpace(c.APIToken)
	if c.TradeType == "" {
		c.TradeType = "usdt.trc20"
	}
	if c.Fiat == "" {
		c.Fiat = "CNY"
	}
	if c.Timeout <= 0 {
		c.Timeout = 1200
	}
	if c.Timeout < 120 {
		c.Timeout = 120
	}
	return c, nil
}

func (g BEpusdtGateway) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (g BEpusdtGateway) CreatePayment(ctx context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	cfg, err := parseBepusdtConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("bepusdt: base_url 未配置（部署 BEpusdt 后在支付方式中填写）")
	}
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("bepusdt: api_token 未配置")
	}
	if opts.NotifyURL == "" || opts.RedirectURL == "" {
		return nil, fmt.Errorf("bepusdt: 缺少 notify_url/redirect_url（请配置 site_url 或传入 return_url）")
	}

	amountStr := payment.FormatFiatFromCents(order.TotalAmount)
	// Signature params must match request field values as strings
	signParams := map[string]string{
		"order_id":     order.TradeNo,
		"amount":       amountStr,
		"notify_url":   opts.NotifyURL,
		"redirect_url": opts.RedirectURL,
		"trade_type":   cfg.TradeType,
		"fiat":         cfg.Fiat,
		"name":         order.PlanName,
		"timeout":      strconv.Itoa(cfg.Timeout),
	}
	sig := payment.SignMD5(signParams, cfg.APIToken)

	// JSON body: amount as number for API
	amountNum, _ := strconv.ParseFloat(amountStr, 64)
	body := map[string]any{
		"order_id":     order.TradeNo,
		"amount":       amountNum,
		"notify_url":   opts.NotifyURL,
		"redirect_url": opts.RedirectURL,
		"trade_type":   cfg.TradeType,
		"fiat":         cfg.Fiat,
		"name":         order.PlanName,
		"timeout":      cfg.Timeout,
		"signature":    sig,
	}
	raw, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/api/v1/order/create-transaction", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("bepusdt request failed: %w（请确认 BEpusdt 已部署且 base_url 可达）", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID        string `json:"trade_id"`
			OrderID        string `json:"order_id"`
			Amount         string `json:"amount"`
			ActualAmount   string `json:"actual_amount"`
			Token          string `json:"token"`
			ExpirationTime int64  `json:"expiration_time"`
			PaymentURL     string `json:"payment_url"`
			Status         any    `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("bepusdt invalid response: %s", truncate(string(respBody), 200))
	}
	if result.StatusCode != 200 {
		msg := result.Message
		if msg == "" {
			msg = string(respBody)
		}
		return nil, fmt.Errorf("bepusdt error: %s", msg)
	}
	if result.Data.PaymentURL == "" {
		return nil, fmt.Errorf("bepusdt: empty payment_url")
	}

	expireAt := time.Now().Unix() + result.Data.ExpirationTime
	if result.Data.ExpirationTime <= 0 {
		expireAt = order.ExpiredAt.Unix()
	}

	return &payment.PaymentIntent{
		Type:       payment.IntentRedirect,
		URL:        result.Data.PaymentURL,
		PayAddress: result.Data.Token,
		TradeNo:    order.TradeNo,
		Amount:     order.TotalAmount,
		Currency:   order.Currency,
		Message:    fmt.Sprintf("请使用 USDT 支付 %s（约 %s）", amountStr, result.Data.ActualAmount),
		ExpireAt:   expireAt,
		Extra: map[string]any{
			"trade_id":      result.Data.TradeID,
			"actual_amount": result.Data.ActualAmount,
			"token":         result.Data.Token,
			"payment_url":   result.Data.PaymentURL,
			"gateway":       "bepusdt",
		},
	}, nil
}

// notifyPayload matches BEpusdt async notify JSON.
type notifyPayload struct {
	TradeID            string  `json:"trade_id"`
	OrderID            string  `json:"order_id"`
	Amount             any     `json:"amount"` // number or string
	ActualAmount       any     `json:"actual_amount"`
	Token              string  `json:"token"`
	BlockTransactionID string  `json:"block_transaction_id"`
	Signature          string  `json:"signature"`
	Status             any     `json:"status"` // 1 wait 2 success 3 timeout
}

func (g BEpusdtGateway) HandleNotify(_ context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	cfg, err := parseBepusdtConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("bepusdt: api_token missing")
	}

	var p notifyPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("invalid notify body")
	}

	status := anyToInt(p.Status)
	amountStr := anyToAmountString(p.Amount)
	actualStr := anyToAmountString(p.ActualAmount)

	signParams := map[string]string{
		"trade_id":             p.TradeID,
		"order_id":             p.OrderID,
		"amount":               amountStr,
		"actual_amount":        actualStr,
		"token":                p.Token,
		"block_transaction_id": p.BlockTransactionID,
		"status":               strconv.Itoa(status),
	}
	// Only non-empty participate — SignMD5 already skips empty
	expect := payment.SignMD5(signParams, cfg.APIToken)
	if !strings.EqualFold(expect, strings.TrimSpace(p.Signature)) {
		return nil, fmt.Errorf("bepusdt: invalid signature")
	}

	// status: 1 waiting — ignore for fulfill; 2 success; 3 timeout
	if status != 2 {
		return &payment.NotifyResult{
			TradeNo: p.OrderID,
			Success: false,
			Raw:     string(body),
		}, nil
	}

	paidCents := payment.FiatToCents(anyToFloat(p.Amount))
	return &payment.NotifyResult{
		TradeNo:    p.OrderID,
		PaidAmount: paidCents,
		Success:    true,
		CallbackNo: firstNonEmpty(p.BlockTransactionID, p.TradeID),
		Raw:        string(body),
	}, nil
}

func (g BEpusdtGateway) QueryPayment(_ context.Context, order *models.Order, _ string) (*payment.QueryResult, error) {
	// BEpusdt public query not required for Phase 2a; admin mark-paid remains available.
	return &payment.QueryResult{
		Paid:       order.Status == models.OrderPaid,
		PaidAmount: order.TotalAmount,
		CallbackNo: order.CallbackNo,
	}, nil
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

func anyToFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	case int:
		return float64(t)
	case int64:
		return float64(t)
	default:
		return 0
	}
}

func anyToAmountString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// Prefer integer form when exact; else full float string
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		s := fmt.Sprint(v)
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func init() {
	payment.Register(BEpusdtGateway{})
}
