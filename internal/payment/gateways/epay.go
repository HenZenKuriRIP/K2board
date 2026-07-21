package gateways

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"K2board/internal/models"
	"K2board/internal/payment"
)

// EpayGateway integrates Rainbow / common 易支付 (V1 MD5).
// Create: page jump via {base}/submit.php
// Notify: GET/POST form params, ACK body "success"
// Query:  GET {base}/api.php?act=order&pid=&key=&out_trade_no=
//
// Config JSON on payment_methods.config:
//
//	{
//	  "base_url": "https://pay.example.com",
//	  "pid": "1001",
//	  "key": "merchant_secret",
//	  "type": "",                 // empty = epay cashier; alipay|wxpay|...
//	  "product_name": "数字商品"   // optional; supports {plan_name} {trade_no} {trade_tail}
//	}
type EpayGateway struct {
	HTTPClient *http.Client
}

func (EpayGateway) Code() string { return "epay" }
func (EpayGateway) Name() string { return "彩虹易支付" }

type epayConfig struct {
	BaseURL     string `json:"base_url"`
	PID         string `json:"pid"`
	Key         string `json:"key"`
	Type        string `json:"type"`         // pay type; empty → cashier
	ProductName string `json:"product_name"` // goods title template
	// Legacy aliases accepted for convenience
	APIToken string `json:"api_token"`
	Secret   string `json:"secret"`
}

func parseEpayConfig(cfg string) (epayConfig, error) {
	var c epayConfig
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		return c, fmt.Errorf("epay: invalid config JSON: %w", err)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.PID = strings.TrimSpace(c.PID)
	c.Key = strings.TrimSpace(c.Key)
	if c.Key == "" {
		c.Key = strings.TrimSpace(c.APIToken)
	}
	if c.Key == "" {
		c.Key = strings.TrimSpace(c.Secret)
	}
	c.Type = strings.ToLower(strings.TrimSpace(c.Type))
	// Normalize "auto/cashier" to empty (epay own cashier)
	switch c.Type {
	case "auto", "cashier", "any", "select":
		c.Type = ""
	}
	c.ProductName = strings.TrimSpace(c.ProductName)
	if c.ProductName == "" {
		c.ProductName = "数字商品"
	}
	return c, nil
}

func (c epayConfig) validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("epay: base_url 未配置")
	}
	if c.PID == "" {
		return fmt.Errorf("epay: pid 未配置")
	}
	if c.Key == "" {
		return fmt.Errorf("epay: key 未配置（商户密钥）")
	}
	return nil
}

func (g EpayGateway) client() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// epaySign implements standard 易支付 MD5:
// sort non-empty params (except sign/sign_type) by key, join k=v&..., md5(raw+KEY) lowercase.
func epaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + key
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func epayProductName(tmpl string, order *models.Order) string {
	name := tmpl
	trade := ""
	plan := ""
	if order != nil {
		trade = order.TradeNo
		plan = order.PlanName
	}
	tail := trade
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	name = strings.ReplaceAll(name, "{trade_no}", trade)
	name = strings.ReplaceAll(name, "{trade_tail}", tail)
	name = strings.ReplaceAll(name, "{plan_name}", plan)
	// 易支付 name 超长会截断；避免控制字符
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "数字商品"
	}
	// ~127 bytes limit in docs; keep runes reasonable
	if utf8.RuneCountInString(name) > 64 {
		runes := []rune(name)
		name = string(runes[:64])
	}
	return name
}

func (g EpayGateway) CreatePayment(_ context.Context, order *models.Order, cfgJSON string, opts payment.CreateOptions) (*payment.PaymentIntent, error) {
	cfg, err := parseEpayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if opts.NotifyURL == "" {
		return nil, fmt.Errorf("epay: 缺少 notify_url（请在系统设置填写 site_url）")
	}
	if opts.RedirectURL == "" {
		return nil, fmt.Errorf("epay: 缺少 return_url（请配置 site_url 或传入 return_url）")
	}

	money := payment.FormatFiatFromCents(order.TotalAmount)
	params := map[string]string{
		"pid":          cfg.PID,
		"out_trade_no": order.TradeNo,
		"notify_url":   opts.NotifyURL,
		"return_url":   opts.RedirectURL,
		"name":         epayProductName(cfg.ProductName, order),
		"money":        money,
	}
	if cfg.Type != "" {
		params["type"] = cfg.Type
	}
	params["sign"] = epaySign(params, cfg.Key)
	params["sign_type"] = "MD5"

	// Build submit.php query (GET redirect — widely supported)
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	payURL := cfg.BaseURL + "/submit.php?" + q.Encode()

	return &payment.PaymentIntent{
		Type:     payment.IntentRedirect,
		URL:      payURL,
		TradeNo:  order.TradeNo,
		Amount:   order.TotalAmount,
		Currency: order.Currency,
		Message:  "将跳转易支付收银台完成付款",
		ExpireAt: order.ExpiredAt.Unix(),
		Extra: map[string]any{
			"payment_url": payURL,
			"gateway":     "epay",
			"type":        cfg.Type,
			"money":       money,
		},
	}, nil
}

// HandleNotify accepts form-urlencoded or query-string body (notify handler normalizes GET → body).
func (g EpayGateway) HandleNotify(_ context.Context, _ map[string]string, body []byte, cfgJSON string) (*payment.NotifyResult, error) {
	cfg, err := parseEpayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	params, err := parseEpayNotifyParams(body)
	if err != nil {
		return nil, err
	}

	gotSign := strings.TrimSpace(params["sign"])
	if gotSign == "" {
		return nil, fmt.Errorf("epay: missing sign")
	}
	expect := epaySign(params, cfg.Key)
	if !strings.EqualFold(expect, gotSign) {
		return nil, fmt.Errorf("epay: invalid signature")
	}

	// Optional pid check (defense in depth)
	if pid := strings.TrimSpace(params["pid"]); pid != "" && pid != cfg.PID {
		return nil, fmt.Errorf("epay: pid mismatch")
	}

	outTradeNo := strings.TrimSpace(params["out_trade_no"])
	if outTradeNo == "" {
		return nil, fmt.Errorf("epay: missing out_trade_no")
	}

	status := strings.TrimSpace(params["trade_status"])
	success := status == "TRADE_SUCCESS" || status == "success" || status == "SUCCESS"
	moneyStr := strings.TrimSpace(params["money"])
	paidCents := int64(0)
	if moneyStr != "" {
		f, err := strconv.ParseFloat(moneyStr, 64)
		if err != nil {
			return nil, fmt.Errorf("epay: invalid money %q", moneyStr)
		}
		paidCents = payment.FiatToCents(f)
	}

	callbackNo := strings.TrimSpace(params["trade_no"])
	if callbackNo == "" {
		callbackNo = outTradeNo
	}

	return &payment.NotifyResult{
		TradeNo:    outTradeNo,
		PaidAmount: paidCents,
		Success:    success,
		CallbackNo: callbackNo,
		Raw:        string(body),
	}, nil
}

func parseEpayNotifyParams(body []byte) (map[string]string, error) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return nil, fmt.Errorf("epay: empty notify body")
	}
	// Prefer form/query encoding
	vals, err := url.ParseQuery(raw)
	if err == nil && len(vals) > 0 {
		out := make(map[string]string, len(vals))
		for k, vs := range vals {
			if len(vs) > 0 {
				out[k] = vs[0]
			}
		}
		// Heuristic: if we got sign or out_trade_no, treat as form
		if out["sign"] != "" || out["out_trade_no"] != "" || out["trade_status"] != "" {
			return out, nil
		}
	}
	// Fallback JSON (some forks)
	var m map[string]any
	if json.Unmarshal(body, &m) == nil && len(m) > 0 {
		out := make(map[string]string, len(m))
		for k, v := range m {
			if v == nil {
				continue
			}
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("epay: invalid notify encoding: %w", err)
	}
	return nil, fmt.Errorf("epay: cannot parse notify body")
}

// QueryPayment uses 易支付 api.php?act=order
func (g EpayGateway) QueryPayment(ctx context.Context, order *models.Order, cfgJSON string) (*payment.QueryResult, error) {
	cfg, err := parseEpayConfig(cfgJSON)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	u, err := url.Parse(cfg.BaseURL + "/api.php")
	if err != nil {
		return nil, fmt.Errorf("epay: bad base_url: %w", err)
	}
	q := u.Query()
	q.Set("act", "order")
	q.Set("pid", cfg.PID)
	q.Set("key", cfg.Key)
	q.Set("out_trade_no", order.TradeNo)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("epay query failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		Code       any    `json:"code"`
		Msg        string `json:"msg"`
		TradeNo    string `json:"trade_no"`
		OutTradeNo string `json:"out_trade_no"`
		Money      string `json:"money"`
		Status     any    `json:"status"` // 1 paid, 0 unpaid
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("epay query invalid response: %s", truncate(string(body), 200))
	}
	code := anyToInt(result.Code)
	if code != 1 {
		// Order not found / not paid yet — soft miss for reconcile
		return &payment.QueryResult{Paid: false}, nil
	}
	st := anyToInt(result.Status)
	if st != 1 {
		return &payment.QueryResult{Paid: false}, nil
	}
	paid := order.TotalAmount
	if result.Money != "" {
		if f, e := strconv.ParseFloat(result.Money, 64); e == nil {
			paid = payment.FiatToCents(f)
		}
	}
	cb := strings.TrimSpace(result.TradeNo)
	if cb == "" {
		cb = order.CallbackNo
	}
	return &payment.QueryResult{
		Paid:       true,
		PaidAmount: paid,
		CallbackNo: cb,
	}, nil
}

func init() {
	payment.Register(EpayGateway{})
}
